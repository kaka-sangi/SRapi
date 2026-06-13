package service

import (
	"context"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const settingsKeyOpsSettings = "admin_control.ops_settings"

func (s *Service) GetOpsSettings(ctx context.Context) (admincontrol.OpsSettings, error) {
	settings := defaultOpsSettings()
	if err := s.loadTyped(ctx, settingsKeyOpsSettings, &settings); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	return settings, nil
}

func (s *Service) UpdateOpsSettings(ctx context.Context, settings admincontrol.OpsSettings, actorUserID int) (admincontrol.OpsSettings, error) {
	if err := validateOpsSettings(settings); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyOpsSettings, settings, actorUserID); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	return settings, nil
}

func defaultOpsSettings() admincontrol.OpsSettings {
	return admincontrol.OpsSettings{
		AutoRefreshEnabled:     true,
		RefreshIntervalSeconds: 15,
	}
}

func validateOpsSettings(settings admincontrol.OpsSettings) error {
	if settings.RefreshIntervalSeconds < 5 || settings.RefreshIntervalSeconds > 3600 {
		return admincontrol.ErrInvalidInput
	}
	return nil
}
