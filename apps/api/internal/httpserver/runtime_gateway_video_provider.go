package httpserver

import (
	"context"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func (rt *runtimeState) invokeProviderVideo(ctx context.Context, req provideradaptercontract.VideoRequest) (provideradaptercontract.VideoResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.VideoResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeVideo(ctx, req)
	if err != nil {
		if refreshed, retried := rt.retryAfterAuthRefresh(ctx, req.Account, dispatch.credential, err); retried {
			req.Credential = refreshed
			resp, err = rt.adapters.InvokeVideo(ctx, req)
		}
		if err != nil {
			rt.applyProviderAccountProtection(ctx, req.Account, err)
			return provideradaptercontract.VideoResponse{}, err
		}
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderVideoContent(ctx context.Context, req provideradaptercontract.VideoRequest) (provideradaptercontract.VideoContentResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.VideoContentResponse{}, err
	}
	releaseLeases := true
	defer func() {
		if releaseLeases {
			rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
		}
	}()
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeVideoContent(ctx, req)
	if err != nil {
		if refreshed, retried := rt.retryAfterAuthRefresh(ctx, req.Account, dispatch.credential, err); retried {
			req.Credential = refreshed
			resp, err = rt.adapters.InvokeVideoContent(ctx, req)
		}
		if err != nil {
			rt.applyProviderAccountProtection(ctx, req.Account, err)
			return provideradaptercontract.VideoContentResponse{}, err
		}
	}
	if resp.Content != nil {
		leases := dispatch.concurrencyLeases
		resp.Content = newStreamLeaseCloser(resp.Content, func() {
			rt.releaseGatewayConcurrency(leases)
		})
		releaseLeases = false
	}
	return resp, nil
}

func gatewayUsageFromVideoProvider(resp provideradaptercontract.VideoResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayVideoFromProvider(resp provideradaptercontract.VideoResponse) gatewaycontract.VideoResponse {
	var err *gatewaycontract.VideoError
	if resp.Error != nil {
		err = &gatewaycontract.VideoError{
			Code:     resp.Error.Code,
			Message:  resp.Error.Message,
			Metadata: cloneAnyMap(resp.Error.Metadata),
		}
	}
	return gatewaycontract.VideoResponse{
		ID:          resp.ID,
		Model:       resp.Model,
		Status:      gatewaycontract.VideoStatus(resp.Status),
		Progress:    cloneIntPtr(resp.Progress),
		Prompt:      resp.Prompt,
		Seconds:     cloneIntPtr(resp.Seconds),
		Size:        resp.Size,
		CreatedAt:   cloneInt64Ptr(resp.CreatedAt),
		CompletedAt: cloneInt64Ptr(resp.CompletedAt),
		ExpiresAt:   cloneInt64Ptr(resp.ExpiresAt),
		Error:       err,
		Metadata:    cloneAnyMap(resp.Metadata),
	}
}
