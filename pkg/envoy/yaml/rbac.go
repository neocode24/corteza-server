package yaml

import (
	"github.com/cortezaproject/corteza-server/pkg/rbac"
	"gopkg.in/yaml.v3"
)

type (
	rbacRules struct {
		// mapping unresolved roles to defined RBAC rules
		rules map[string]rbac.RuleSet
	}
)

// Decodes RBAC rules defined as access/role/ops...
func (wrap *rbacRules) DecodeResourceRules(res rbac.Resource, k, v *yaml.Node) error {
	var (
		access rbac.Access
	)

	if wrap.rules == nil {
		wrap.rules = make(map[string]rbac.RuleSet)
	}

	if !isKind(v, yaml.MappingNode) {
		return nodeErr(v, "rbac per access rule definition must be a map")
	}

	if k.Value == "allow" {
		access = rbac.Allow
	}

	return iterator(v, func(k, v *yaml.Node) error {

		var (
			role = k.Value
		)

		if _, set := wrap.rules[role]; !set {
			wrap.rules[role] = rbac.RuleSet{}
		}

		if !isKind(v, yaml.SequenceNode) {
			return nodeErr(v, "rbac rule operations list must be a sequence")
		}

		return iterator(v, func(_, v *yaml.Node) error {
			rule := &rbac.Rule{Resource: res, Operation: rbac.Operation(v.Value), Access: access}
			wrap.rules[role] = append(wrap.rules[role], rule)
			return nil
		})
	})
}

// Decodes RBAC rules defined as access/role/resource/ops...
func (wrap *rbacRules) DecodeGlobalRules(k, v *yaml.Node) error {
	var (
		access rbac.Access
	)

	if wrap.rules == nil {
		wrap.rules = make(map[string]rbac.RuleSet)
	}

	if !isKind(v, yaml.MappingNode) {
		return nodeErr(v, "rbac per access rule definition must be a map")
	}

	if k.Value == "allow" {
		access = rbac.Allow
	}

	return iterator(v, func(k, v *yaml.Node) error {

		var (
			role = k.Value
		)

		if _, set := wrap.rules[role]; !set {
			wrap.rules[role] = rbac.RuleSet{}
		}

		if !isKind(v, yaml.MappingNode) {
			return nodeErr(v, "rbac per role rule definition must be a map")
		}

		return iterator(v, func(k, v *yaml.Node) error {
			var res = rbac.Resource(k.Value)
			if !res.IsValid() {
				return nodeErr(k, "rbac resource %s invalid", k.Value)
			}

			if !isKind(v, yaml.SequenceNode) {
				return nodeErr(v, "rbac rule operations list must be a sequence")
			}

			return iterator(v, func(_, v *yaml.Node) error {
				rule := &rbac.Rule{Resource: res, Operation: rbac.Operation(v.Value), Access: access}
				wrap.rules[role] = append(wrap.rules[role], rule)
				return nil
			})
		})
	})
}
