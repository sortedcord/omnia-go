package sim

import (
	"fmt"
	"slices"
	"strings"
)

// Visibility defines the ACL exposure level of an attribute.
type Visibility string

const (
	VisibilityPublic  Visibility = "public"  // Any entity in context can perceive this attribute
	VisibilityPrivate Visibility = "private" // Restricted to Arbiter and entities in Accessors list
)

// Attribute represents a dynamic somatic, psychological, or physical property on an entity.
type Attribute struct {
	Value      string     `json:"value"`
	Allowed    []string   `json:"allowed,omitempty"`   // Non-empty indicates an enum
	Core       bool       `json:"core,omitempty"`      // Core attributes cannot be removed
	Visibility Visibility `json:"visibility"`          // "public" or "private"
	Accessors  []EntityID `json:"accessors,omitempty"` // Whitelist of entity IDs who can observe this when private
}

// CanAccess reports whether viewerID has permission to observe this attribute.
func (a Attribute) CanAccess(viewerID EntityID) bool {
	if a.Visibility == VisibilityPublic {
		return true
	}
	// For private attributes, only entities explicitly in Accessors have access.
	// If Accessors is empty, NOBODY (not even the owner entity) can observe it.
	return slices.Contains(a.Accessors, viewerID)
}

// Attributes is an ACL-aware collection of attributes belonging to an entity.
type Attributes map[string]Attribute

// NewAttributes initializes an empty Attributes map for an entity.
func NewAttributes() Attributes {
	return make(Attributes)
}

// Get retrieves the current string value of an attribute.
func (a Attributes) Get(key string) (string, bool) {
	attr, ok := a[key]
	if !ok {
		return "", false
	}
	return attr.Value, true
}

// Set mutates an attribute value, enforcing enum constraints if configured.
// If the attribute already exists, its visibility and accessors are preserved.
func (a Attributes) Set(key, value string) error {
	attr, exists := a[key]
	if exists && len(attr.Allowed) > 0 {
		valid := false
		for _, allowed := range attr.Allowed {
			if strings.EqualFold(allowed, value) {
				valid = true
				value = allowed
				break
			}
		}
		if !valid {
			return fmt.Errorf("attributes: invalid value %q for enum attribute %q (allowed: %v)", value, key, attr.Allowed)
		}
	}

	if exists {
		attr.Value = value
		a[key] = attr
	} else {
		a[key] = Attribute{
			Value:      value,
			Visibility: VisibilityPublic,
		}
	}
	return nil
}

// SetWithVisibility sets value, visibility, and accessors atomically.
// Per design rule: if visibility is public, the accessors list is strictly cleared out.
func (a Attributes) SetWithVisibility(key, value string, vis Visibility, accessors []EntityID) error {
	if vis == "" {
		vis = VisibilityPublic
	}
	if vis == VisibilityPublic {
		accessors = nil // Always cleared out when public
	}

	attr, exists := a[key]
	if exists && len(attr.Allowed) > 0 {
		valid := false
		for _, allowed := range attr.Allowed {
			if strings.EqualFold(allowed, value) {
				valid = true
				value = allowed
				break
			}
		}
		if !valid {
			return fmt.Errorf("attributes: invalid value %q for enum attribute %q (allowed: %v)", value, key, attr.Allowed)
		}
	}

	core := false
	var allowed []string
	if exists {
		core = attr.Core
		allowed = attr.Allowed
	}

	a[key] = Attribute{
		Value:      value,
		Allowed:    allowed,
		Core:       core,
		Visibility: vis,
		Accessors:  accessors,
	}
	return nil
}

// SetVisibility changes an attribute's visibility and updates its accessor list.
func (a Attributes) SetVisibility(key string, vis Visibility, accessors []EntityID) error {
	attr, exists := a[key]
	if !exists {
		return fmt.Errorf("attributes: attribute %q not found", key)
	}

	if vis == VisibilityPublic {
		attr.Visibility = VisibilityPublic
		attr.Accessors = nil
	} else if vis == VisibilityPrivate {
		attr.Visibility = VisibilityPrivate
		attr.Accessors = accessors
	} else if vis == "" {
		if attr.Visibility == VisibilityPrivate {
			attr.Accessors = accessors
		}
	}

	a[key] = attr
	return nil
}

// Remove deletes an attribute if it is not marked as Core.
func (a Attributes) Remove(key string) error {
	attr, ok := a[key]
	if !ok {
		return nil
	}
	if attr.Core {
		return fmt.Errorf("attributes: cannot remove core attribute %q", key)
	}
	delete(a, key)
	return nil
}

// Copy returns an isolated deep copy of the attributes map.
func (a Attributes) Copy() Attributes {
	out := make(Attributes, len(a))
	for k, v := range a {
		allowedCopy := append([]string(nil), v.Allowed...)
		accessorsCopy := append([]EntityID(nil), v.Accessors...)
		out[k] = Attribute{
			Value:      v.Value,
			Allowed:    allowedCopy,
			Core:       v.Core,
			Visibility: v.Visibility,
			Accessors:  accessorsCopy,
		}
	}
	return out
}

// AccessibleTo returns a key-value map of attributes that viewerID is permitted to observe.
func (a Attributes) AccessibleTo(viewerID EntityID) map[string]string {
	out := make(map[string]string)
	for k, attr := range a {
		if attr.CanAccess(viewerID) {
			out[k] = attr.Value
		}
	}
	return out
}
