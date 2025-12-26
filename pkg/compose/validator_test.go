package compose_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/compose"
)

var _ = Describe("Validator", func() {
	Describe("ValidateCompose", func() {
		Context("with valid compose file", func() {
			It("should return valid result with metadata", func() {
				validCompose := `
version: "3.8"
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
  db:
    image: postgres:13
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:

networks:
  frontend:
  backend:
`

				result, err := compose.ValidateCompose(validCompose)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Metadata).ToNot(BeNil())
				Expect(result.Errors).To(BeEmpty())
			})
		})

		Context("with valid compose file and title", func() {
			It("should extract title from x-lissto extension", func() {
				composeWithTitle := `
version: "3.8"
x-lissto:
  title: "My Application"

services:
  app:
    image: myapp:latest
`

				result, err := compose.ValidateCompose(composeWithTitle)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Metadata.Title).To(Equal("My Application"))
			})
		})

		Context("with invalid YAML", func() {
			It("should return invalid result with errors", func() {
				invalidCompose := `
this is not valid YAML
  - broken
    indentation
`

				result, err := compose.ValidateCompose(invalidCompose)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Errors).ToNot(BeEmpty())
			})
		})

		Context("with undefined environment variables", func() {
			It("should capture warnings", func() {
				composeWithEnvVars := `
version: "3.8"
services:
  web:
    image: webapp:latest
    environment:
      - DB_HOST=${DB_HOST}
      - DB_PORT=${DB_PORT}
`

				result, err := compose.ValidateCompose(composeWithEnvVars)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Warnings).ToNot(BeEmpty())
				Expect(len(result.Warnings)).To(BeNumerically(">=", 2))
			})
		})
	})
})
