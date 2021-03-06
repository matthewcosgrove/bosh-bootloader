package commands

import "github.com/cloudfoundry/bosh-bootloader/storage"

//go:generate counterfeiter -o ./fakes/terraform_applier.go --fake-name TerraformApplier . terraformApplier
type terraformApplier interface {
	ValidateVersion() error
	GetOutputs(storage.State) (map[string]interface{}, error)
	Apply(storage.State) (storage.State, error)
}

type terraformDestroyer interface {
	ValidateVersion() error
	GetOutputs(storage.State) (map[string]interface{}, error)
	Destroy(storage.State) (storage.State, error)
}

type terraformOutputter interface {
	GetOutputs(storage.State) (map[string]interface{}, error)
}
