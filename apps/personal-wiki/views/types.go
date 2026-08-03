package views

// PageSummary is a lightweight row for list views (home, tag, backlinks,
// recently-viewed).
type PageSummary struct {
	ID           int64
	Title        string
	Tags         []string
	UpdatedLabel string
}

// Page is the full record for view/edit.
type Page struct {
	ID              int64
	Title           string
	Body            string // raw, with [[Title]] markup — used for editing
	RenderedBody    string // markdown with [[Title]] resolved to /page/:id links
	Tags            []string
	TagsRaw         string // comma-joined, for the edit form
	CreatedLabel    string
	UpdatedLabel    string
	LastViewedLabel string
}
