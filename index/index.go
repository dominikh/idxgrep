package index

type Statistics struct {
	Indexed int
	Skipped int
}

type Index interface {
	CreateIndex() error
	Index(string) (Statistics, error)
}
