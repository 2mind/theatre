package console

import (
	"io/ioutil"
	"net/http"

	workloadsv1alpha1 "github.com/gocardless/theatre/pkg/apis/workloads/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func mustConsoleAuthorisationFixture(path string) *workloadsv1alpha1.ConsoleAuthorisation {
	consoleAuthorisation := &workloadsv1alpha1.ConsoleAuthorisation{}

	consoleAuthorisationFixtureYAML, _ := ioutil.ReadFile(path)

	decoder := serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
	if err := runtime.DecodeInto(decoder, consoleAuthorisationFixtureYAML, consoleAuthorisation); err != nil {
		admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return consoleAuthorisation
}

var _ = Describe("Authorisation webhook", func() {
	Describe("Validate", func() {
		var (
			updateFixture string
			update        *consoleAuthorisationUpdate
			err           error
		)

		existingAuth := mustConsoleAuthorisationFixture("./testdata/console_authorisation_existing.yaml")

		JustBeforeEach(func() {
			updatedAuth := mustConsoleAuthorisationFixture(updateFixture)
			update = &consoleAuthorisationUpdate{
				existingAuth: existingAuth,
				updatedAuth:  updatedAuth,
				user:         "current-user",
			}

			err = update.Validate()
		})

		Context("Adding a single authoriser", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_add.yaml"
			})

			It("Returns no errors", func() {
				Expect(err).To(BeNil())
			})
		})

		Context("Adding multiple authorisers", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_add_multiple.yaml"
			})

			It("Returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("spec.authorisations field can only be appended to")))
			})
		})

		Context("Adding an authoriser who is another user", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_add_another_user.yaml"
			})

			It("Returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("only the current user can be added as an authoriser")))
			})
		})

		Context("Adding an authoriser who is the console owner", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_add_owner.yaml"
			})

			It("Returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("authoriser cannot authorise their own console")))
			})
		})

		Context("Changing immutable fields", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_immutables.yaml"
			})

			It("Returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("field is immutable")))
			})
		})

		Context("Removing an existing authoriser", func() {
			BeforeEach(func() {
				updateFixture = "./testdata/console_authorisation_update_remove.yaml"
			})

			It("Returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("spec.authorisations field can only be appended to")))
			})
		})
	})
})
