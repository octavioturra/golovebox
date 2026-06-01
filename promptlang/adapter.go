package promptlang

import "github.com/user/golovebox/core"

// ToIntent converts a ParsedSpec to a core.Intent.
func ToIntent(spec *ParsedSpec) core.Intent {
	intent := core.Intent{
		Raw:       spec.Content,
		TechDebts: spec.TechDebts,
	}
	for _, a := range spec.Annotations {
		intent.Annotations = append(intent.Annotations, core.Annotation{
			Keyword:  string(a.Keyword),
			Argument: a.Argument,
			Line:     a.Line,
			Raw:      a.Raw,
		})
	}
	return intent
}
