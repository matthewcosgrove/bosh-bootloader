package terraform_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry/bosh-bootloader/fakes"
	"github.com/cloudfoundry/bosh-bootloader/storage"
	"github.com/cloudfoundry/bosh-bootloader/terraform"
	newFakes "github.com/cloudfoundry/bosh-bootloader/terraform/fakes"
	"github.com/pivotal-cf-experimental/gomegamatchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	var (
		executor              *fakes.TerraformExecutor
		templateGenerator     *fakes.TemplateGenerator
		inputGenerator        *fakes.InputGenerator
		outputGenerator       *fakes.OutputGenerator
		logger                *fakes.Logger
		migrator              *newFakes.StackMigrator
		manager               terraform.Manager
		terraformOutputBuffer bytes.Buffer
		expectedTFState       string
		expectedTFOutput      string
	)

	BeforeEach(func() {
		executor = &fakes.TerraformExecutor{}
		templateGenerator = &fakes.TemplateGenerator{}
		inputGenerator = &fakes.InputGenerator{}
		outputGenerator = &fakes.OutputGenerator{}
		logger = &fakes.Logger{}
		migrator = &newFakes.StackMigrator{}

		expectedTFOutput = "some terraform output"
		expectedTFState = "some-updated-tf-state"

		manager = terraform.NewManager(terraform.NewManagerArgs{
			Executor:              executor,
			TemplateGenerator:     templateGenerator,
			InputGenerator:        inputGenerator,
			AWSOutputGenerator:    outputGenerator,
			GCPOutputGenerator:    outputGenerator,
			TerraformOutputBuffer: &terraformOutputBuffer,
			Logger:                logger,
			StackMigrator:         migrator,
		})
	})

	Describe("Apply", func() {
		var (
			incomingState storage.State
			expectedState storage.State
		)

		BeforeEach(func() {
			incomingState = storage.State{
				IAAS:  "gcp",
				EnvID: "some-env-id",
				GCP: storage.GCP{
					ServiceAccountKey: "some-service-account-key",
					ProjectID:         "some-project-id",
					Zone:              "some-zone",
					Region:            "some-region",
				},
				TFState: "some-tf-state",
				LB: storage.LB{
					Type:   "cf",
					Domain: "some-domain",
				},
			}

			executor.ApplyCall.Returns.TFState = expectedTFState

			expectedState = incomingState
			expectedState.TFState = expectedTFState
			expectedState.LatestTFOutput = expectedTFOutput

			templateGenerator.GenerateCall.Returns.Template = "some-gcp-terraform-template"
			inputGenerator.GenerateCall.Returns.Inputs = map[string]string{
				"env_id":        incomingState.EnvID,
				"project_id":    incomingState.GCP.ProjectID,
				"region":        incomingState.GCP.Region,
				"zone":          incomingState.GCP.Zone,
				"credentials":   "some-path",
				"system_domain": incomingState.LB.Domain,
			}

			migrator.MigrateReturns(incomingState, nil)
		})

		It("logs steps", func() {
			_, err := manager.Apply(storage.State{})
			Expect(err).NotTo(HaveOccurred())

			Expect(logger.StepCall.Messages).To(gomegamatchers.ContainSequence([]string{
				"validating whether stack needs to be migrated",
				"generating terraform template",
				"generating terraform variables",
				"applying terraform template",
			}))
		})

		It("returns a state with new tfState and output from executor apply", func() {
			terraformOutputBuffer.Write([]byte(expectedTFOutput))

			state, err := manager.Apply(incomingState)
			Expect(err).NotTo(HaveOccurred())

			Expect(migrator.MigrateCallCount()).To(Equal(1))
			Expect(migrator.MigrateArgsForCall(0)).To(Equal(incomingState))

			Expect(templateGenerator.GenerateCall.Receives.State).To(Equal(incomingState))

			Expect(inputGenerator.GenerateCall.Receives.State).To(Equal(incomingState))

			Expect(executor.ApplyCall.Receives.Inputs).To(Equal(map[string]string{
				"env_id":        incomingState.EnvID,
				"project_id":    incomingState.GCP.ProjectID,
				"region":        incomingState.GCP.Region,
				"zone":          incomingState.GCP.Zone,
				"credentials":   "some-path",
				"system_domain": incomingState.LB.Domain,
			}))
			Expect(executor.ApplyCall.Receives.TFState).To(Equal("some-tf-state"))
			Expect(executor.ApplyCall.Receives.Template).To(Equal(string("some-gcp-terraform-template")))
			Expect(state).To(Equal(expectedState))
		})

		Context("when an error occurs", func() {
			Context("when the stack cannot be migrated", func() {
				It("returns an error", func() {
					migrator.MigrateReturns(storage.State{}, errors.New("failed to migrate"))

					_, err := manager.Apply(incomingState)
					Expect(err).To(MatchError("failed to migrate"))
				})
			})

			Context("when InputGenerator.Generate returns an error", func() {
				BeforeEach(func() {
					inputGenerator.GenerateCall.Returns.Error = errors.New("failed to generate inputs")
				})

				It("bubbles up the error", func() {
					_, err := manager.Apply(incomingState)
					Expect(err).To(MatchError("failed to generate inputs"))
				})
			})

			Context("when the applying causes an executor error", func() {
				BeforeEach(func() {
					executor.ApplyCall.Returns.Error = &fakes.TerraformExecutorError{}

					terraformOutputBuffer.Write([]byte(expectedTFOutput))
				})

				AfterEach(func() {
					executor.ApplyCall.Returns.Error = nil
				})

				It("returns the bblState with latest terraform output and a ManagerError", func() {
					_, err := manager.Apply(incomingState)

					Expect(err).To(BeAssignableToTypeOf(terraform.ManagerError{}))
				})
			})

			Context("when migrating causes an executor error", func() {
				BeforeEach(func() {
					migrator.MigrateReturns(storage.State{}, &fakes.TerraformExecutorError{})
				})

				It("returns the bblState with latest terraform output and a ManagerError", func() {
					_, err := manager.Apply(incomingState)

					Expect(err).To(BeAssignableToTypeOf(terraform.ManagerError{}))
				})
			})

			Context("when Executor.Apply returns a non-ExecutorError error", func() {
				executorError := errors.New("some-error")

				BeforeEach(func() {
					executor.ApplyCall.Returns.Error = executorError
				})

				AfterEach(func() {
					executor.ApplyCall.Returns.Error = nil
				})

				It("bubbles up the error", func() {
					_, err := manager.Apply(incomingState)
					Expect(err).To(Equal(executorError))
				})
			})
		})
	})

	Describe("Destroy", func() {
		Context("when the bbl state contains a non-empty TFState", func() {
			var (
				incomingState storage.State
				expectedState storage.State
			)

			BeforeEach(func() {
				incomingState = storage.State{
					EnvID: "some-env-id",
					GCP: storage.GCP{
						ServiceAccountKey: "some-service-account-key",
						ProjectID:         "some-project-id",
						Zone:              "some-zone",
						Region:            "some-region",
					},
					LB: storage.LB{
						Type:   "cf",
						Domain: "some-domain",
					},
					TFState: "some-tf-state",
				}
				executor.DestroyCall.Returns.TFState = expectedTFState

				expectedState = incomingState
				expectedState.TFState = expectedTFState
				expectedState.LatestTFOutput = expectedTFOutput

				templateGenerator.GenerateCall.Returns.Template = "some-gcp-terraform-template"
				inputGenerator.GenerateCall.Returns.Inputs = map[string]string{
					"env_id":        incomingState.EnvID,
					"project_id":    incomingState.GCP.ProjectID,
					"region":        incomingState.GCP.Region,
					"zone":          incomingState.GCP.Zone,
					"credentials":   "some-path",
					"system_domain": incomingState.LB.Domain,
				}
			})

			It("logs steps", func() {
				_, err := manager.Destroy(incomingState)
				Expect(err).NotTo(HaveOccurred())

				Expect(logger.StepCall.Messages).To(gomegamatchers.ContainSequence([]string{
					"destroying infrastructure", "finished destroying infrastructure",
				}))
			})

			It("calls Executor.Destroy with the right arguments", func() {
				_, err := manager.Destroy(incomingState)
				Expect(err).NotTo(HaveOccurred())

				Expect(templateGenerator.GenerateCall.Receives.State).To(Equal(incomingState))

				Expect(inputGenerator.GenerateCall.Receives.State).To(Equal(incomingState))

				Expect(executor.DestroyCall.Receives.Inputs).To(Equal(map[string]string{
					"env_id":        incomingState.EnvID,
					"project_id":    incomingState.GCP.ProjectID,
					"region":        incomingState.GCP.Region,
					"zone":          incomingState.GCP.Zone,
					"credentials":   "some-path",
					"system_domain": incomingState.LB.Domain,
				}))
				Expect(executor.DestroyCall.Receives.Template).To(Equal(templateGenerator.GenerateCall.Returns.Template))
				Expect(executor.DestroyCall.Receives.TFState).To(Equal(incomingState.TFState))
			})

			It("returns the bbl state updated with the TFState and output from executor destroy", func() {
				terraformOutputBuffer.Write([]byte(expectedTFOutput))

				newBBLState, err := manager.Destroy(incomingState)
				Expect(err).NotTo(HaveOccurred())

				Expect(newBBLState).To(Equal(expectedState))
			})

			Context("when InputGenerator.Generate returns an error", func() {
				BeforeEach(func() {
					inputGenerator.GenerateCall.Returns.Error = errors.New("failed to generate inputs")
				})

				It("bubbles up the error", func() {
					_, err := manager.Apply(incomingState)
					Expect(err).To(MatchError("failed to generate inputs"))
				})
			})

			Context("when Executor.Destroy returns a ExecutorError", func() {
				var (
					tempDir       string
					executorError *fakes.TerraformExecutorError
				)

				BeforeEach(func() {
					var err error
					tempDir, err = ioutil.TempDir("", "")
					Expect(err).NotTo(HaveOccurred())

					err = ioutil.WriteFile(filepath.Join(tempDir, "terraform.tfstate"), []byte("updated-tf-state"), os.ModePerm)
					Expect(err).NotTo(HaveOccurred())

					executorError = &fakes.TerraformExecutorError{}
					executor.DestroyCall.Returns.Error = executorError

					terraformOutputBuffer.Write([]byte(expectedTFOutput))
				})

				AfterEach(func() {
					executor.DestroyCall.Returns.Error = nil
				})

				It("returns a ManagerError", func() {
					_, err := manager.Destroy(incomingState)

					expectedState := incomingState
					expectedState.LatestTFOutput = expectedTFOutput
					expectedError := terraform.NewManagerError(expectedState, executorError)
					Expect(err).To(MatchError(expectedError))
				})
			})

			Context("when Executor.Destroy returns a non-ExecutorError error", func() {
				executorError := errors.New("some-error")

				BeforeEach(func() {
					executor.DestroyCall.Returns.Error = executorError
				})

				AfterEach(func() {
					executor.DestroyCall.Returns.Error = nil
				})

				It("bubbles up the error", func() {
					_, err := manager.Destroy(incomingState)
					Expect(err).To(Equal(executorError))
				})
			})
		})
		Context("when the bbl state contains a non-empty TFState", func() {
			var (
				incomingState = storage.State{EnvID: "some-env-id"}
			)
			It("returns the bbl state and skips calling executor destroy", func() {
				bblState, err := manager.Destroy(incomingState)
				Expect(err).NotTo(HaveOccurred())

				Expect(bblState).To(Equal(incomingState))
				Expect(executor.DestroyCall.CallCount).To(Equal(0))
			})
		})
	})

	Describe("GetOutputs", func() {
		BeforeEach(func() {
			outputGenerator.GenerateCall.Returns.Outputs = map[string]interface{}{
				"external_ip": "some-external-ip",
			}
		})

		It("returns all terraform outputs except lb related outputs", func() {
			incomingState := storage.State{
				IAAS:    "gcp",
				TFState: "some-tf-state",
			}

			terraformOutputs, err := manager.GetOutputs(incomingState)
			Expect(err).NotTo(HaveOccurred())

			Expect(outputGenerator.GenerateCall.Receives.TFState).To(Equal("some-tf-state"))
			Expect(terraformOutputs).To(Equal(map[string]interface{}{
				"external_ip": "some-external-ip",
			}))
		})

		Context("when the output generator fails", func() {
			It("returns the error to the caller", func() {
				outputGenerator.GenerateCall.Returns.Error = errors.New("fail")
				_, err := manager.GetOutputs(storage.State{
					IAAS: "gcp",
				})
				Expect(err).To(MatchError("fail"))
			})
		})
	})

	Describe("Version", func() {
		BeforeEach(func() {
			executor.VersionCall.Returns.Version = "some-version"
		})

		It("returns a version", func() {
			version, err := manager.Version()
			Expect(err).NotTo(HaveOccurred())

			Expect(executor.VersionCall.CallCount).To(Equal(1))
			Expect(version).To(Equal("some-version"))
		})

		Context("when executor version returns an error", func() {
			BeforeEach(func() {
				executor.VersionCall.Returns.Error = errors.New("failed to get version")
			})

			It("returns the error", func() {
				_, err := manager.Version()
				Expect(err).To(MatchError("failed to get version"))
			})
		})
	})

	Describe("ValidateVersion", func() {
		Context("when terraform version is greater than v0.8.5", func() {
			BeforeEach(func() {
				executor.VersionCall.Returns.Version = "0.9.1"
			})

			It("validates the version of terraform and returns no error", func() {
				err := manager.ValidateVersion()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when terraform version is v0.9.0", func() {
			BeforeEach(func() {
				executor.VersionCall.Returns.Version = "0.9.0"
			})

			It("returns a helpful error message", func() {
				err := manager.ValidateVersion()
				Expect(err).To(MatchError("Version 0.9.0 of terraform is incompatible with bbl, please try a later version."))
			})
		})

		Context("failure cases", func() {
			It("returns an error when the terraform installed is less than v0.8.5", func() {
				executor.VersionCall.Returns.Version = "0.8.4"

				err := manager.ValidateVersion()
				Expect(err).To(MatchError("Terraform version must be at least v0.8.5"))
			})

			It("fast fails if the terraform executor fails to get the version", func() {
				executor.VersionCall.Returns.Error = errors.New("cannot get version")

				err := manager.ValidateVersion()
				Expect(err).To(MatchError("cannot get version"))
			})

			It("fast fails when the version cannot be parsed by go-semver", func() {
				executor.VersionCall.Returns.Version = "lol.5.2"

				err := manager.ValidateVersion()
				Expect(err.Error()).To(ContainSubstring("invalid syntax"))
			})
		})
	})
})
