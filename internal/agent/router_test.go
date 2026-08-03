package agent

import (
	"strings"
	"testing"
)

func TestRouterValidateAcceptsNamedAndDefaultChildren(t *testing.T) {
	t.Parallel()

	resource := validRouterResource()
	resource.Spec.Children = []Child{
		{Ref: "roles/default-key", Route: "default"},
		{Ref: "roles/fallback", Default: true},
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("Resource.Validate() error: %v", err)
	}
}

func TestRouterValidateRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Resource)
		want   string
	}{
		{
			name: "missing route template",
			mutate: func(resource *Resource) {
				resource.Spec.Route = ""
			},
			want: "validate schema",
		},
		{
			name: "blank route template",
			mutate: func(resource *Resource) {
				resource.Spec.Route = " "
			},
			want: "Router requires nonblank spec.route",
		},
		{
			name: "invalid route template",
			mutate: func(resource *Resource) {
				resource.Spec.Route = "{{"
			},
			want: "spec.route",
		},
		{
			name: "no children",
			mutate: func(resource *Resource) {
				resource.Spec.Children = nil
			},
			want: "validate schema",
		},
		{
			name: "child without edge selector",
			mutate: func(resource *Resource) {
				resource.Spec.Children = []Child{{Ref: "roles/worker"}}
			},
			want: "validate schema",
		},
		{
			name: "child with route and default",
			mutate: func(resource *Resource) {
				resource.Spec.Children = []Child{{Ref: "roles/worker", Route: "work", Default: true}}
			},
			want: "validate schema",
		},
		{
			name: "route with surrounding whitespace",
			mutate: func(resource *Resource) {
				resource.Spec.Children = []Child{{Ref: "roles/worker", Route: " work "}}
			},
			want: "must not have leading or trailing whitespace",
		},
		{
			name: "duplicate named route",
			mutate: func(resource *Resource) {
				resource.Spec.Children = []Child{
					{Ref: "roles/first", Route: "work"},
					{Ref: "roles/second", Route: "work"},
				}
			},
			want: "duplicates spec.children[0].route",
		},
		{
			name: "multiple defaults",
			mutate: func(resource *Resource) {
				resource.Spec.Children = []Child{
					{Ref: "roles/first", Default: true},
					{Ref: "roles/second", Default: true},
				}
			},
			want: "duplicates spec.children[0].default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := validRouterResource()
			test.mutate(&resource)

			err := resource.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resource.Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestNonRouterValidateRejectsRouterChildFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		child Child
	}{
		{name: "route", child: Child{Ref: "roles/worker", Route: "work"}},
		{name: "default", child: Child{Ref: "roles/worker", Default: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := Resource{
				APIVersion: APIVersion,
				Kind:       SequentialKind,
				ID:         "workflows/pipeline",
				Spec: Spec{
					Description: "pipeline",
					Children:    []Child{test.child},
					Body:        "{{ .Input }}",
				},
			}

			err := resource.Validate()
			if err == nil || !strings.Contains(err.Error(), "validate schema") {
				t.Fatalf("Resource.Validate() error = %v, want schema rejection", err)
			}
		})
	}
}

func validRouterResource() Resource {
	return Resource{
		APIVersion: APIVersion,
		Kind:       RouterKind,
		ID:         "workflows/router",
		Source:     "workflows/router.yaml",
		Spec: Spec{
			Description: "Routes work",
			Route:       "{{ .Input }}",
			Children:    []Child{{Ref: "roles/worker", Route: "work"}},
			Body:        "{{ .Prompt }}",
		},
	}
}
