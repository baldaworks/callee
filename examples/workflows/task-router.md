---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes one classified task to exactly one handler.
  route: '{{ .Input }}'
  children:
    - ref: roles/implementer
      alias: routed_implementer
      route: implement
    - ref: roles/reviewer
      alias: routed_reviewer
      route: review
    - ref: roles/explorer
      alias: routed_generalist
      default: true
---
{{ .Prompt }}
