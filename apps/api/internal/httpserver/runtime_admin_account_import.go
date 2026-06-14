package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleImportAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.ProviderAccountImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account import request", requestID)
		return
	}
	createdIDs := make([]apiopenapi.Id, 0)
	updatedIDs := make([]apiopenapi.Id, 0)
	importErrors := make([]string, 0)
	items := make([]apiopenapi.CodexSessionImportItem, 0, len(body.Accounts))
	warnings := make([]apiopenapi.CodexSessionImportMessage, 0)
	skipped := 0
	failed := 0
	seen := map[string]int{}
	existing := s.buildAccountImportIndex(r.Context())
	for idx, item := range body.Accounts {
		index := idx + 1
		providerID, err := strconv.Atoi(string(item.ProviderId))
		if err != nil || providerID <= 0 {
			failed++
			message := fmt.Sprintf("accounts[%d].provider_id invalid", idx)
			importErrors = append(importErrors, message)
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
			continue
		}
		provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
		if err != nil {
			failed++
			message := fmt.Sprintf("accounts[%d].provider_id not found", idx)
			importErrors = append(importErrors, message)
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
			continue
		}
		credential := derefMap(item.Credential)
		if len(credential) == 0 {
			failed++
			message := fmt.Sprintf("accounts[%d].credential required", idx)
			importErrors = append(importErrors, message)
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
			continue
		}
		metadata := jsonObjectToMap(item.Metadata)
		identityKeys := buildImportIdentityKeys(providerID, item.Name, accountcontract.RuntimeClass(item.RuntimeClass), item.UpstreamClient, metadata, credential)
		if dup, ok := firstSeenImportIdentity(seen, identityKeys); ok {
			message := fmt.Sprintf("duplicate of import entry #%d; skipped", dup)
			skipped++
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionSkipped, nil, message))
			warnings = append(warnings, apiopenapi.CodexSessionImportMessage{Index: index, Name: ptrString(item.Name), Message: message})
			continue
		}
		markImportIdentitySeen(seen, identityKeys, index)
		if existingID, ok := existing.find(identityKeys); ok {
			credential, err = s.refreshImportCredential(r.Context(), accountcontract.RuntimeClass(item.RuntimeClass), item.UpstreamClient, metadata, item.ProxyId, credential)
			if err != nil {
				failed++
				message := fmt.Sprintf("accounts[%d] oauth refresh failed", idx)
				importErrors = append(importErrors, message)
				items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
				continue
			}
			updated, err := s.updateImportedAccount(r.Context(), existingID, item, metadata, credential)
			if err != nil {
				failed++
				message := fmt.Sprintf("accounts[%d] update failed", idx)
				importErrors = append(importErrors, message)
				items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, idPtr(existingID), message))
				continue
			}
			updatedID := apiopenapi.Id(strconv.Itoa(updated.ID))
			updatedIDs = append(updatedIDs, updatedID)
			existing.add(updated.ID, buildImportIdentityKeys(updated.ProviderID, updated.Name, updated.RuntimeClass, updated.UpstreamClient, updated.Metadata, nil))
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionUpdated, &updatedID, ""))
			s.addImportedAccountGroups(r.Context(), idx, index, item, updated.ID, &importErrors, &warnings)
			continue
		}
		credential, err = s.refreshImportCredential(r.Context(), accountcontract.RuntimeClass(item.RuntimeClass), item.UpstreamClient, metadata, item.ProxyId, credential)
		if err != nil {
			failed++
			message := fmt.Sprintf("accounts[%d] oauth refresh failed", idx)
			importErrors = append(importErrors, message)
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
			continue
		}
		account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
			ProviderID:     providerID,
			Name:           item.Name,
			RuntimeClass:   accountcontract.RuntimeClass(item.RuntimeClass),
			Credential:     credential,
			Metadata:       applyProviderTemplateMetadata(provider, metadata),
			ProxyID:        item.ProxyId,
			Status:         toAccountStatusPtr(item.Status),
			Priority:       item.Priority,
			Weight:         item.Weight,
			RiskLevel:      stringPtrFromAPI(item.RiskLevel),
			UpstreamClient: item.UpstreamClient,
		})
		if err != nil {
			failed++
			message := fmt.Sprintf("accounts[%d] create failed", idx)
			importErrors = append(importErrors, message)
			items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionFailed, nil, message))
			continue
		}
		createdID := apiopenapi.Id(strconv.Itoa(account.ID))
		createdIDs = append(createdIDs, createdID)
		existing.add(account.ID, identityKeys)
		items = append(items, importResultItem(index, item.Name, apiopenapi.CodexSessionImportItemActionCreated, &createdID, ""))
		s.addImportedAccountGroups(r.Context(), idx, index, item, account.ID, &importErrors, &warnings)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.import", "provider_account", "bulk", nil, map[string]any{
		"created_count": len(createdIDs),
		"updated_count": len(updatedIDs),
		"skipped_count": skipped,
		"failed_count":  failed,
		"warning_count": len(warnings),
		"error_count":   len(importErrors),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountImportResponse{
		Data: apiopenapi.ProviderAccountImportResult{
			CreatedCount: len(createdIDs),
			CreatedIds:   createdIDs,
			Errors:       importErrors,
			FailedCount:  failed,
			Items:        items,
			SkippedCount: skipped,
			TotalCount:   len(body.Accounts),
			UpdatedCount: len(updatedIDs),
			UpdatedIds:   updatedIDs,
			Warnings:     warnings,
		},
		RequestId: requestID,
	})
}

func (s *Server) updateImportedAccount(ctx context.Context, accountID int, item apiopenapi.ProviderAccountImportItem, metadata map[string]any, credential map[string]any) (accountcontract.ProviderAccount, error) {
	runtimeClass := accountcontract.RuntimeClass(item.RuntimeClass)
	proxyID := item.ProxyId
	upstreamClient := item.UpstreamClient
	return s.runtime.accounts.Update(ctx, accountID, accountcontract.UpdateRequest{
		Name:           &item.Name,
		RuntimeClass:   &runtimeClass,
		Credential:     &credential,
		Metadata:       &metadata,
		ProxyID:        &proxyID,
		Status:         toAccountStatusPtr(item.Status),
		Priority:       item.Priority,
		Weight:         item.Weight,
		RiskLevel:      stringPtrFromAPI(item.RiskLevel),
		UpstreamClient: &upstreamClient,
	})
}

func (s *Server) addImportedAccountGroups(ctx context.Context, idx int, index int, item apiopenapi.ProviderAccountImportItem, accountID int, importErrors *[]string, warnings *[]apiopenapi.CodexSessionImportMessage) {
	groupIDs, err := apiIDsToInts(item.GroupIds)
	if err != nil {
		message := fmt.Sprintf("accounts[%d].group_ids invalid", idx)
		*importErrors = append(*importErrors, message)
		*warnings = append(*warnings, apiopenapi.CodexSessionImportMessage{Index: index, Name: ptrString(item.Name), Message: message})
		return
	}
	for _, groupID := range groupIDs {
		if _, err := s.runtime.accounts.AddAccountToGroup(ctx, accountID, groupID); err != nil {
			message := fmt.Sprintf("accounts[%d].group_ids[%d] add failed", idx, groupID)
			*importErrors = append(*importErrors, message)
			*warnings = append(*warnings, apiopenapi.CodexSessionImportMessage{Index: index, Name: ptrString(item.Name), Message: message})
		}
	}
}

func importResultItem(index int, name string, action apiopenapi.CodexSessionImportItemAction, accountID *apiopenapi.Id, message string) apiopenapi.CodexSessionImportItem {
	item := apiopenapi.CodexSessionImportItem{Index: index, Action: action, AccountId: accountID}
	if strings.TrimSpace(name) != "" {
		item.Name = ptrString(name)
	}
	if strings.TrimSpace(message) != "" {
		item.Message = ptrString(message)
	}
	return item
}

func (s *Server) buildAccountImportIndex(ctx context.Context) *importIdentityIndex {
	index := newImportIdentityIndex()
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return index
	}
	for _, account := range accounts {
		keys := buildImportIdentityKeys(account.ProviderID, account.Name, account.RuntimeClass, account.UpstreamClient, account.Metadata, nil)
		index.add(account.ID, keys)
	}
	return index
}
