package pushrules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ActionsToTweaks converts a list of actions into a primary action
// kind and a tweaks map. Returns a nil map if it would have been
// empty.
func ActionsToTweaks(as []*Action) (ActionKind, map[string]interface{}, error) {
	var kind ActionKind
	var tweaks map[string]interface{}

	for _, a := range as {
		switch a.Kind {
		case DontNotifyAction:
			// Don't bother processing any further
			return DontNotifyAction, nil, nil

		case SetTweakAction:
			if tweaks == nil {
				tweaks = map[string]interface{}{}
			}
			tweaks[string(a.Tweak)] = a.Value

		default:
			if kind != UnknownAction {
				return UnknownAction, nil, fmt.Errorf("got multiple primary actions: already had %q, got %s", kind, a.Kind)
			}
			kind = a.Kind
		}
	}

	return kind, tweaks, nil
}

// BoolTweakOr returns the named tweak as a boolean, and returns `def`
// on failure.
func BoolTweakOr(tweaks map[string]interface{}, key TweakKey, def bool) bool {
	v, ok := tweaks[string(key)]
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

// globToRegexp converts a Matrix glob-style pattern to a Regular expression.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	// TODO: It's unclear which glob characters are supported. The only
	// place this is discussed is for the unrelated "m.policy.rule.*"
	// events. Assuming, the same: /[*?]/
	if !strings.ContainsAny(pattern, "*?") {
		pattern = "*" + pattern + "*"
	}

	// The defined syntax doesn't allow escaping the glob wildcard
	// characters, which makes this a straight-forward
	// replace-after-quote.
	pattern = globNonMetaRegexp.ReplaceAllStringFunc(pattern, regexp.QuoteMeta)
	pattern = strings.Replace(pattern, "*", ".*", -1)
	pattern = strings.Replace(pattern, "?", ".", -1)
	return regexp.Compile("^(" + pattern + ")$")
}

// globNonMetaRegexp are the characters that are not considered glob
// meta-characters (i.e. may need escaping).
var globNonMetaRegexp = regexp.MustCompile("[^*?]+")

// lookupMapPath traverses a hierarchical map structure, like the one
// produced by json.Unmarshal, to return the leaf value. Traversing
// arrays/slices is not supported, only objects/maps.
func lookupMapPath(path []string, m map[string]interface{}) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	var v interface{} = m
	for i, key := range path {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected an object for path %q, but got %T", strings.Join(path[:i+1], "."), v)
		}

		v, ok = m[key]
		if !ok {
			return nil, fmt.Errorf("path not found: %s", strings.Join(path[:i+1], "."))
		}
	}

	return v, nil
}

// parseRoomMemberCountCondition parses a string like "2", "==2", "<2"
// into a function that checks if the argument to it fulfils the
// condition.
func parseRoomMemberCountCondition(s string) (func(int) bool, error) {
	var b int
	var cmp = func(a int) bool { return a == b }
	switch {
	case strings.HasPrefix(s, "<="):
		cmp = func(a int) bool { return a <= b }
		s = s[2:]
	case strings.HasPrefix(s, ">="):
		cmp = func(a int) bool { return a >= b }
		s = s[2:]
	case strings.HasPrefix(s, "<"):
		cmp = func(a int) bool { return a < b }
		s = s[1:]
	case strings.HasPrefix(s, ">"):
		cmp = func(a int) bool { return a > b }
		s = s[1:]
	case strings.HasPrefix(s, "=="):
		// Same cmp as the default.
		s = s[2:]
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	b = int(v)
	return cmp, nil
}
