// Package asr provides provider-neutral transcription routing and sequencing.
package asr

import (
	"fmt"

	"github.com/tossp/voxink/internal/domain"
)

// AsrVendor identifies a stable ASR supplier independently from its models.
type AsrVendor string

const (
	// VendorVolcengine identifies the Volcengine supplier.
	VendorVolcengine AsrVendor = "volcengine"
	// VendorMiMo identifies the MiMo supplier.
	VendorMiMo AsrVendor = "mimo"
	// VendorMOSI identifies the MOSI supplier.
	VendorMOSI AsrVendor = "mosi"
)

// VendorDescriptor declares the models owned by one supplier.
type VendorDescriptor struct {
	Vendor AsrVendor
	Models []domain.ProviderKind
}

// Registry stores stable supplier descriptors without constructing adapters.
type Registry struct {
	descriptors map[AsrVendor]VendorDescriptor
}

// NewRegistry validates and copies supplier descriptors.
func NewRegistry(descriptors ...VendorDescriptor) (Registry, error) {
	registry := Registry{descriptors: make(map[AsrVendor]VendorDescriptor, len(descriptors))}
	for _, descriptor := range descriptors {
		if descriptor.Vendor == "" {
			return Registry{}, fmt.Errorf("ASR vendor must not be empty")
		}
		if _, exists := registry.descriptors[descriptor.Vendor]; exists {
			return Registry{}, fmt.Errorf("duplicate ASR vendor %q", descriptor.Vendor)
		}
		descriptor.Models = append([]domain.ProviderKind(nil), descriptor.Models...)
		registry.descriptors[descriptor.Vendor] = descriptor
	}
	return registry, nil
}

// DefaultRegistry returns the built-in supplier-to-model associations.
func DefaultRegistry() Registry {
	registry, err := NewRegistry(
		VendorDescriptor{
			Vendor: VendorVolcengine,
			Models: []domain.ProviderKind{domain.ProviderVolcengineV3},
		},
		VendorDescriptor{
			Vendor: VendorMiMo,
			Models: []domain.ProviderKind{domain.ProviderMiMoASR, domain.ProviderMiMoV25},
		},
		VendorDescriptor{
			Vendor: VendorMOSI,
			Models: []domain.ProviderKind{domain.ProviderMOSI},
		},
	)
	if err != nil {
		panic("invalid built-in ASR registry: " + err.Error())
	}
	return registry
}

// Lookup returns a copied descriptor for a supplier.
func (r Registry) Lookup(vendor AsrVendor) (VendorDescriptor, bool) {
	descriptor, ok := r.descriptors[vendor]
	if !ok {
		return VendorDescriptor{}, false
	}
	descriptor.Models = append([]domain.ProviderKind(nil), descriptor.Models...)
	return descriptor, true
}

// Route selects one primary supplier and an optional, different backup.
type Route struct {
	Primary AsrVendor
	Backup  AsrVendor
}

// Validate checks that the route refers to registered, distinct suppliers.
func (r Route) Validate(registry Registry) error {
	if r.Primary == "" {
		return fmt.Errorf("primary ASR vendor must not be empty")
	}
	if _, ok := registry.Lookup(r.Primary); !ok {
		return fmt.Errorf("primary ASR vendor %q is not registered", r.Primary)
	}
	if r.Backup == "" {
		return nil
	}
	if r.Backup == r.Primary {
		return fmt.Errorf("backup ASR vendor must differ from primary %q", r.Primary)
	}
	if _, ok := registry.Lookup(r.Backup); !ok {
		return fmt.Errorf("backup ASR vendor %q is not registered", r.Backup)
	}
	return nil
}

// RouteProvider binds one supplier to the model selected from that supplier.
type RouteProvider struct {
	Vendor AsrVendor
	Model  domain.ProviderKind
}

// ProviderRoute selects the model-specific primary and backup providers.
type ProviderRoute struct {
	Primary RouteProvider
	Backup  RouteProvider
}

// StageOneRoute returns the fixed stage-one Volcengine live and MiMo batch route.
func StageOneRoute() ProviderRoute {
	return ProviderRoute{
		Primary: RouteProvider{Vendor: VendorVolcengine, Model: domain.ProviderVolcengineV3},
		Backup:  RouteProvider{Vendor: VendorMiMo, Model: domain.ProviderMiMoASR},
	}
}

// ValidateStageOneRoute checks that route is exactly the supported stage-one route.
func ValidateStageOneRoute(route ProviderRoute, registry Registry) error {
	if route != StageOneRoute() {
		return fmt.Errorf("stage-one ASR route must use Volcengine V3 primary and MiMo mimo-v2.5-asr backup")
	}
	return route.Validate(registry)
}

// Validate checks supplier separation and model ownership for a provider route.
func (r ProviderRoute) Validate(registry Registry) error {
	vendors := Route{Primary: r.Primary.Vendor, Backup: r.Backup.Vendor}
	if err := vendors.Validate(registry); err != nil {
		return err
	}
	if r.Backup.Vendor == "" {
		return fmt.Errorf("backup ASR vendor must not be empty")
	}
	if err := validateModelOwnership(registry, "primary", r.Primary); err != nil {
		return err
	}
	return validateModelOwnership(registry, "backup", r.Backup)
}

func validateModelOwnership(registry Registry, role string, provider RouteProvider) error {
	if provider.Model == "" {
		return fmt.Errorf("%s ASR model must not be empty", role)
	}
	descriptor, _ := registry.Lookup(provider.Vendor)
	for _, model := range descriptor.Models {
		if model == provider.Model {
			return nil
		}
	}
	return fmt.Errorf("%s ASR model %q does not belong to vendor %q", role, provider.Model, provider.Vendor)
}
