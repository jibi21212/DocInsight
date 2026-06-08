package pdf

type Page struct {
	Number int
	Text   string
}

type ExtractResult struct {
	Pages     []Page
	PageCount int
}

type Extractor interface {
	Extract(data []byte) (*ExtractResult, error)
}
