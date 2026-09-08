package configuration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/config"
)

type Resolution struct {
	Mode               Mode
	EffectiveConfig    config.Config
	Prefill            RuntimeConfig
	Snapshot           *Snapshot
	ConfigurationError string
}

type Resolver struct {
	repository *Repository
	now        func() time.Time
}

func NewResolver(repository *Repository) *Resolver {
	return &Resolver{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (resolver *Resolver) Resolve(ctx context.Context, bootstrap config.BootstrapConfig, legacy config.Config) Resolution {
	snapshot, err := resolver.repository.Latest(ctx)
	if err == nil {
		return resolver.resolveSnapshot(ctx, bootstrap, snapshot, legacy)
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		return Resolution{Mode: ModeConfigurationError, ConfigurationError: err.Error()}
	}

	prefill, legacyErr := FromLegacyConfig(legacy)
	if legacyErr == nil {
		if validationErrs := Validate(prefill); len(validationErrs) == 0 {
			saved, saveErr := resolver.repository.Save(ctx, StatusReady, SourceEnvImport, prefill)
			if saveErr != nil {
				return Resolution{Mode: ModeConfigurationError, Prefill: prefill, ConfigurationError: saveErr.Error()}
			}
			return resolver.resolveSnapshot(ctx, bootstrap, saved, legacy)
		}
	}
	result := Resolution{Mode: ModeSetupPending, Prefill: prefill}
	if legacyErr != nil {
		result.ConfigurationError = legacyErr.Error()
	}
	return result
}

func (resolver *Resolver) resolveSnapshot(ctx context.Context, bootstrap config.BootstrapConfig, snapshot *Snapshot, legacy config.Config) Resolution {
	snapshot.Config = ApplyDefaults(snapshot.Config)
	if validationErrs := Validate(snapshot.Config); len(validationErrs) > 0 {
		return Resolution{
			Mode:               ModeConfigurationError,
			Prefill:            snapshot.Config,
			Snapshot:           snapshot,
			ConfigurationError: fmt.Sprintf("runtime configuration validation failed: %v", validationErrs),
		}
	}
	if snapshot.Status == StatusRestartRequired {
		if err := resolver.repository.MarkApplied(ctx, snapshot.Revision, resolver.now()); err != nil {
			return Resolution{Mode: ModeConfigurationError, Prefill: snapshot.Config, Snapshot: snapshot, ConfigurationError: err.Error()}
		}
		refreshed, err := resolver.repository.Latest(ctx)
		if err != nil {
			return Resolution{Mode: ModeConfigurationError, Prefill: snapshot.Config, Snapshot: snapshot, ConfigurationError: err.Error()}
		}
		snapshot = refreshed
	} else if snapshot.Status != StatusReady {
		return Resolution{
			Mode:               ModeConfigurationError,
			Prefill:            snapshot.Config,
			Snapshot:           snapshot,
			ConfigurationError: fmt.Sprintf("unsupported runtime configuration status %q", snapshot.Status),
		}
	}
	effective, err := ToEffectiveConfig(bootstrap, snapshot.Config)
	if err != nil {
		return Resolution{Mode: ModeConfigurationError, Prefill: snapshot.Config, Snapshot: snapshot, ConfigurationError: err.Error()}
	}
	effective.AgentRunner.ProviderConfigRevision = snapshot.Revision
	// 控制面是基础设施配置（与 bootstrap 同级），不进快照 schema。ready
	// 产物必须回填 legacy，否则生产永远落到内存库且不建种子超管。
	effective.ControlPlane = legacy.ControlPlane
	return Resolution{
		Mode:            ModeReady,
		EffectiveConfig: effective,
		Prefill:         snapshot.Config,
		Snapshot:        snapshot,
	}
}
