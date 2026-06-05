package scheduledtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
)

const defaultHistoryLimit = 5

// BuildProber constructs a prober for one provider account. The worker supplies
// the real generative prober; tests supply a fake.
type BuildProber func(provider providercontract.Provider, model string) accountcontract.AccountProber

// Runner executes one scheduled-test plan against its account scope, reusing
// accounts.ProbeAccount, recording a run, and applying auto-recover.
type Runner struct {
	accounts  *accountservice.Service
	providers *providerservice.Service
	plans     *scheduledservice.Service
	policy    accountcontract.AccountProbePolicy
	build     BuildProber
}

// NewRunner wires the plan runner. build constructs the per-account prober.
func NewRunner(accounts *accountservice.Service, providers *providerservice.Service, plans *scheduledservice.Service, build BuildProber) (*Runner, error) {
	if accounts == nil || providers == nil || plans == nil || build == nil {
		return nil, accountservice.ErrInvalidInput
	}
	return &Runner{
		accounts:  accounts,
		providers: providers,
		plans:     plans,
		policy:    accountcontract.AccountProbePolicy{HistoryLimit: defaultHistoryLimit},
		build:     build,
	}, nil
}

// RunPlan executes one plan, records the outcome, and returns the recorded run.
func (r *Runner) RunPlan(ctx context.Context, plan scheduledcontract.Plan, trigger string) (scheduledcontract.Run, error) {
	startedAt := time.Now().UTC()
	accounts, err := r.selectAccounts(ctx, plan)
	if err != nil {
		return scheduledcontract.Run{}, err
	}
	outcome := scheduledcontract.RunOutcome{
		Trigger:   trigger,
		Selected:  len(accounts),
		StartedAt: startedAt,
	}
	var firstErr error
	for _, account := range accounts {
		// Inactive accounts are normally skipped. When auto-recover is enabled
		// they are still probed so a recovered upstream can be flipped active.
		if account.Status != accountcontract.StatusActive && !plan.AutoRecover {
			outcome.Skipped++
			continue
		}
		provider, err := r.providers.FindByID(ctx, account.ProviderID)
		if err != nil {
			outcome.Failed++
			firstErr = errors.Join(firstErr, err)
			continue
		}
		model := probeModel(account, provider)
		if model == "" {
			outcome.Skipped++
			continue
		}
		r.probeOne(ctx, plan, account, provider, model, &outcome, &firstErr)
	}
	outcome.FinishedAt = time.Now().UTC()
	outcome.Status = classifyStatus(outcome)
	outcome.Summary = summarize(outcome)
	run, recordErr := r.plans.RecordOutcome(ctx, plan.ID, outcome)
	if recordErr != nil {
		return scheduledcontract.Run{}, recordErr
	}
	return run, firstErr
}

func (r *Runner) probeOne(ctx context.Context, plan scheduledcontract.Plan, account accountcontract.ProviderAccount, provider providercontract.Provider, model string, outcome *scheduledcontract.RunOutcome, firstErr *error) {
	prober := r.build(provider, model)
	snapshot, _, err := r.accounts.ProbeAccount(ctx, account.ID, prober, r.policy)
	if err != nil {
		outcome.Failed++
		*firstErr = errors.Join(*firstErr, err)
		return
	}
	outcome.Probed++
	healthy := strings.EqualFold(snapshot.Status, "healthy") && snapshot.CircuitState != "open"
	if !healthy {
		outcome.Unhealthy++
		return
	}
	if plan.AutoRecover && account.Status != accountcontract.StatusActive {
		if _, err := r.accounts.Recover(ctx, account.ID); err != nil {
			*firstErr = errors.Join(*firstErr, err)
			return
		}
		outcome.Recovered++
	}
}

func (r *Runner) selectAccounts(ctx context.Context, plan scheduledcontract.Plan) ([]accountcontract.ProviderAccount, error) {
	var accounts []accountcontract.ProviderAccount
	switch plan.ScopeType {
	case scheduledcontract.ScopeAccount:
		if plan.ScopeID == nil {
			return nil, scheduledservice.ErrInvalidInput
		}
		account, err := r.accounts.FindByID(ctx, *plan.ScopeID)
		if err != nil {
			return nil, err
		}
		accounts = []accountcontract.ProviderAccount{account}
	case scheduledcontract.ScopeGroup:
		if plan.ScopeID == nil {
			return nil, scheduledservice.ErrInvalidInput
		}
		members, err := r.accounts.ListGroupMembers(ctx, *plan.ScopeID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			account, err := r.accounts.FindByID(ctx, member.AccountID)
			if err != nil {
				continue
			}
			accounts = append(accounts, account)
		}
	default:
		all, err := r.accounts.List(ctx)
		if err != nil {
			return nil, err
		}
		accounts = all
	}
	if plan.MaxResults > 0 && len(accounts) > plan.MaxResults {
		accounts = accounts[:plan.MaxResults]
	}
	return accounts, nil
}

func classifyStatus(outcome scheduledcontract.RunOutcome) string {
	if outcome.Failed > 0 && outcome.Probed == 0 {
		return scheduledcontract.RunStatusFailed
	}
	if outcome.Failed > 0 || outcome.Unhealthy > 0 {
		return scheduledcontract.RunStatusPartial
	}
	return scheduledcontract.RunStatusOK
}

func summarize(outcome scheduledcontract.RunOutcome) string {
	return fmt.Sprintf("probed=%d unhealthy=%d failed=%d skipped=%d recovered=%d",
		outcome.Probed, outcome.Unhealthy, outcome.Failed, outcome.Skipped, outcome.Recovered)
}

// RealProber builds the production generative prober for a provider account.
func RealProber(adapter conversationAdapter) BuildProber {
	return func(provider providercontract.Provider, model string) accountcontract.AccountProber {
		return conversationProber{adapter: adapter, provider: provider, model: model}
	}
}
