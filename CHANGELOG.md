# Changelog

## 0.2.0 (Unreleased)

### Breaking changes

- `pluto_policy` now takes one `definition`: the policy as JSON, in the shape the dashboard's policy
  editor reads and writes. Every policy type the editor saves can be managed, including MCPs, Skills
  and Runtime Enforcement. Pluto validates the definition with the editor's own rules and decides
  whether a stored policy matches it; the provider passes it through unchanged.
- The 0.1 resource is now `pluto_policy_legacy`, unchanged. To keep a 0.1 policy, rename its resource
  and add `moved { from = pluto_policy.<name>  to = pluto_policy_legacy.<name> }` (Terraform 1.8 or later).

## 0.1.1

### Added

- `pluto_policy` resource: create, read, update, delete and import Pluto policies
  (`name`, `category`, `policy_type`, `enforcement_tool`, `tools`, `targeted_scopes`,
  `description`, `custom_message`, `enabled`, `notify_end_user`).
