package promptlang

import "github.com/user/golovebox/core"

// ToIntent converts a ParsedSpec to a core.Intent, mapping DSL annotations to Steps.
func ToIntent(spec *ParsedSpec) core.Intent {
	intent := core.Intent{
		Source:    spec.FilePath,
		Raw:       spec.Content,
		TechDebts: spec.TechDebts,
	}
	anns := spec.Annotations
	for i := 0; i < len(anns); i++ {
		a := anns[i]
		if a.Keyword == KwTry {
			step := core.Step{Kind: core.KindTryElse, Text: a.Argument, Line: a.Line}
			if i+1 < len(anns) && anns[i+1].Keyword == KwOrElse {
				step.OrElse = anns[i+1].Argument
				i++
			}
			intent.Steps = append(intent.Steps, step)
		} else {
			intent.Steps = append(intent.Steps, annotationToStep(a))
		}
	}
	return intent
}

func annotationToStep(a Annotation) core.Step {
	switch a.Keyword {
	case KwNewBranch:
		return core.Step{Kind: core.KindBranch, Title: a.Argument, Line: a.Line}
	case KwPush:
		return core.Step{Kind: core.KindPush, Line: a.Line}
	case KwPR:
		return core.Step{Kind: core.KindPR, Title: a.Argument, Line: a.Line}
	case KwAttentionHere, KwPauseToReview:
		return core.Step{Kind: core.KindCheckpoint, Text: a.Argument, Line: a.Line}
	case KwRunTest:
		return core.Step{Kind: core.KindTest, Text: a.Argument, Line: a.Line}
	case KwNotifyMe:
		return core.Step{Kind: core.KindNotify, Text: a.Argument, Line: a.Line}
	case KwWhen:
		return core.Step{Kind: core.KindWait, Text: a.Argument, Line: a.Line}
	case KwNotTodo:
		return core.Step{Kind: core.KindNotTodo, Text: a.Argument, Line: a.Line}
	default:
		return core.Step{Kind: core.KindEdit, Text: a.Argument, Line: a.Line}
	}
}
