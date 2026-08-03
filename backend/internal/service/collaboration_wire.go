package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
)

func ProvideCollaborationService(
	repository collaborationservice.Repository,
	cfg *config.Config,
	billingCache *BillingCacheService,
	authCache APIKeyAuthCacheInvalidator,
	presence collaborationservice.PresenceStore,
	eventBus collaborationservice.EventBus,
	payloads collaborationservice.PayloadStore,
) (*collaborationservice.Service, error) {
	service, err := collaborationservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		return nil, err
	}
	service.SetChargeCacheInvalidators(billingCache, authCache)
	service.SetPresenceStore(presence)
	service.SetRealtime(eventBus, payloads)
	return service, nil
}

func ProvideCollaborationSweeper(
	repository collaborationservice.Repository,
	cfg *config.Config,
) *collaborationservice.Sweeper {
	sweeper := collaborationservice.NewSweeper(repository, cfg.Collaboration)
	sweeper.Start()
	return sweeper
}
