package rtk

type Pipeline struct {
	Enabled     bool
	MaxOutputChars int
}

func NewPipeline(enabled bool, maxOutputChars int) *Pipeline {
	if maxOutputChars <= 0 {
		maxOutputChars = 8000
	}
	return &Pipeline{
		Enabled:     enabled,
		MaxOutputChars: maxOutputChars,
	}
}

func (p *Pipeline) Process(content string) string {
	if !p.Enabled {
		return content
	}
	if len(content) < 200 {
		return content
	}

	outputType := DetectOutputType(content)

	return p.applyFilter(content, outputType)
}

func (p *Pipeline) ProcessToolResult(content string) string {
	return p.Process(content)
}

func (p *Pipeline) applyFilter(content string, outputType OutputType) string {
	defer func() {
		recover()
	}()

	switch outputType {
	case TypeGitDiff:
		return FilterGitDiff(content, p.MaxOutputChars)
	case TypeGitStatus:
		return FilterGitStatus(content, p.MaxOutputChars)
	case TypeGrep:
		return FilterGrep(content, p.MaxOutputChars)
	case TypeFind, TypeLs:
		return FilterPath(content, p.MaxOutputChars)
	case TypeReadNumbered:
		return FilterSmartTruncate(content, p.MaxOutputChars)
	case TypeSearchList:
		return FilterPath(content, p.MaxOutputChars)
	case TypeDedupLog:
		return FilterDedupLog(content, p.MaxOutputChars)
	case TypeSmartTruncate:
		return FilterSmartTruncate(content, p.MaxOutputChars)
	default:
		return content
	}
}
