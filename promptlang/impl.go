package promptlang

import "github.com/user/golovebox/core"

type impl struct{}

// New returns a core.Parser backed by the promptlang DSL parser.
func New() core.Parser { return &impl{} }

func (p *impl) Parse(src string) (core.Intent, error) {
	spec, err := ParseContent(src)
	if err != nil {
		return core.Intent{}, err
	}
	return ToIntent(spec), nil
}
