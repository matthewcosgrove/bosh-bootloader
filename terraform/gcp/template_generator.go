package gcp

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/storage"
)

type TemplateGenerator struct{}

const backendBase = `resource "google_compute_backend_service" "router-lb-backend-service" {
  name        = "${var.env_id}-router-lb"
  port_name   = "https"
  protocol    = "HTTPS"
  timeout_sec = 900
  enable_cdn  = false
%s
  health_checks = ["${google_compute_health_check.cf-public-health-check.self_link}"]
}
`

func NewTemplateGenerator() TemplateGenerator {
	return TemplateGenerator{}
}

func (t TemplateGenerator) Generate(state storage.State) string {
	template := strings.Join([]string{VarsTemplate, BOSHDirectorTemplate}, "\n")

	switch state.LB.Type {
	case "concourse":
		template = strings.Join([]string{template, ConcourseLBTemplate}, "\n")
	case "cf":
		instanceGroups := t.GenerateInstanceGroups(state.GCP.Zones)
		backendService := t.GenerateBackendService(state.GCP.Zones)

		template = strings.Join([]string{template, CFLBTemplate, instanceGroups, backendService}, "\n")

		if state.LB.Domain != "" {
			template = strings.Join([]string{template, CFDNSTemplate}, "\n")
		}
	}
	switch state.Jumpbox.Enabled {
	case true:
		template = strings.Join([]string{template, JumpboxTemplate}, "\n")
	case false:
		template = strings.Join([]string{template, NonJumpboxTemplate}, "\n")
	}
	return template
}

func (t TemplateGenerator) GenerateBackendService(zoneList []string) string {
	var backends string
	for i := 0; i < len(zoneList); i++ {
		backends = fmt.Sprintf(`%s
  backend {
    group = "${google_compute_instance_group.router-lb-%d.self_link}"
  }
`, backends, i)
	}

	return fmt.Sprintf(backendBase, backends)
}

func (t TemplateGenerator) GenerateInstanceGroups(zoneList []string) string {
	var groups []string
	for i, zone := range zoneList {
		groups = append(groups, fmt.Sprintf(`resource "google_compute_instance_group" "router-lb-%[1]d" {
  name        = "${var.env_id}-router-lb-%[1]d-%[2]s"
  description = "terraform generated instance group that is multi-zone for https loadbalancing"
  zone        = "%[2]s"

  named_port {
    name = "https"
    port = "443"
  }
}
`, i, zone))
	}

	return strings.Join(groups, "\n")
}
