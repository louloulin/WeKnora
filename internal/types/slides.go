// Package types — Build #44 Slide data model.
//
// A SlideDeck is a presentation built from one or more slides. The
// "document → slides" one-click generation reuses the existing PPT
// shape editor (Build #31.2 pptxShapeAdapter) for round-tripping.
//
// Storage: GORM-friendly struct + JSON-friendly struct in one file.
// Migration: 000049_slides.sql (sqlite + mysql).
package types

import (
	"errors"
	"time"
)

// SlideLayout enumerates the supported slide layouts.
type SlideLayout string

const (
	SlideLayoutTitle     SlideLayout = "title"     // Title only
	SlideLayoutSection   SlideLayout = "section"   // Section divider
	SlideLayoutBullet    SlideLayout = "bullet"    // Title + bullets
	SlideLayoutTwoCol    SlideLayout = "two_col"   // Title + 2 columns
	SlideLayoutImage     SlideLayout = "image"     // Title + hero image
	SlideLayoutQuote     SlideLayout = "quote"     // Quote callout
	SlideLayoutEnd       SlideLayout = "end"       // Closing slide
)

// ValidSlideLayouts is the closed set enforced at the API edge.
var ValidSlideLayouts = map[SlideLayout]bool{
	SlideLayoutTitle:   true,
	SlideLayoutSection: true,
	SlideLayoutBullet:  true,
	SlideLayoutTwoCol:  true,
	SlideLayoutImage:   true,
	SlideLayoutQuote:   true,
	SlideLayoutEnd:     true,
}

// SlideTheme enumerates the supported themes.
type SlideTheme string

const (
	SlideThemeNotion      SlideTheme = "notion"
	SlideThemeConfluence  SlideTheme = "confluence"
	SlideThemeCoda        SlideTheme = "coda"
	SlideThemeLark        SlideTheme = "lark"
	SlideThemeApple       SlideTheme = "apple"
	SlideThemeGoogle      SlideTheme = "google"
	SlideThemeAcademic    SlideTheme = "academic"
	SlideThemeDark        SlideTheme = "dark"
)

// ValidSlideThemes is the closed set enforced at the API edge.
var ValidSlideThemes = map[SlideTheme]bool{
	SlideThemeNotion:     true,
	SlideThemeConfluence: true,
	SlideThemeCoda:       true,
	SlideThemeLark:       true,
	SlideThemeApple:      true,
	SlideThemeGoogle:     true,
	SlideThemeAcademic:   true,
	SlideThemeDark:       true,
}

// SlideDeck is the top-level container.
type SlideDeck struct {
	ID           string       `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID     uint64       `json:"tenant_id" gorm:"index"`
	Title        string       `json:"title" gorm:"type:varchar(255)"`
	Theme        SlideTheme   `json:"theme" gorm:"type:varchar(32);default:'notion'"`
	SourceDocID  string       `json:"source_doc_id" gorm:"type:varchar(36);index"`
	KBID         string       `json:"kb_id" gorm:"type:varchar(36);index"`
	OwnerUserID  uint64       `json:"owner_user_id"`
	Visibility   string       `json:"visibility" gorm:"type:varchar(16);default:'private'"`
	SlideCount   int          `json:"slide_count" gorm:"default:0"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// TableName returns the GORM table name.
func (SlideDeck) TableName() string { return "slide_decks" }

// Validate enforces non-empty invariants.
func (d SlideDeck) Validate() error {
	if d.TenantID == 0 {
		return ErrSlideInvalid("tenant_id is required")
	}
	if d.Title == "" {
		return ErrSlideInvalid("title is required")
	}
	if d.Theme != "" && !ValidSlideThemes[d.Theme] {
		return ErrSlideInvalid("theme is invalid")
	}
	if d.OwnerUserID == 0 {
		return ErrSlideInvalid("owner_user_id is required")
	}
	return nil
}

// Slide is a single slide inside a deck.
type Slide struct {
	ID         string      `json:"id" gorm:"primaryKey;type:varchar(36)"`
	DeckID     string      `json:"deck_id" gorm:"type:varchar(36);index"`
	Index      int         `json:"index"`
	Layout     SlideLayout `json:"layout" gorm:"type:varchar(32);default:'bullet'"`
	Title      string      `json:"title" gorm:"type:varchar(512)"`
	Body       string      `json:"body" gorm:"type:text"`
	Bullets    string      `json:"bullets" gorm:"type:text"` // JSON-encoded array
	LeftCol    string      `json:"left_col" gorm:"type:text"`
	RightCol   string      `json:"right_col" gorm:"type:text"`
	ImageURL   string      `json:"image_url" gorm:"type:varchar(1024)"`
	QuoteText  string      `json:"quote_text" gorm:"type:text"`
	QuoteAttr  string      `json:"quote_attr" gorm:"type:varchar(255)"`
	Notes      string      `json:"notes" gorm:"type:text"`
	Background string      `json:"background" gorm:"type:varchar(255)"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// TableName returns the GORM table name.
func (Slide) TableName() string { return "slides" }

// Validate enforces non-empty invariants.
func (s Slide) Validate() error {
	if s.DeckID == "" {
		return ErrSlideInvalid("deck_id is required")
	}
	if s.Layout != "" && !ValidSlideLayouts[s.Layout] {
		return ErrSlideInvalid("layout is invalid")
	}
	return nil
}

// CreateSlideDeckRequest is the body for POST /slides.
type CreateSlideDeckRequest struct {
	Title       string     `json:"title" binding:"required"`
	Theme       SlideTheme `json:"theme"`
	SourceDocID string     `json:"source_doc_id"`
	KBID        string     `json:"kb_id"`
	Visibility  string     `json:"visibility"`
	Slides      []CreateSlideRequest `json:"slides"` // optional seed
}

// CreateSlideRequest is a single slide seed.
type CreateSlideRequest struct {
	Layout     SlideLayout `json:"layout"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	Bullets    []string    `json:"bullets"`
	LeftCol    string      `json:"left_col"`
	RightCol   string      `json:"right_col"`
	ImageURL   string      `json:"image_url"`
	QuoteText  string      `json:"quote_text"`
	QuoteAttr  string      `json:"quote_attr"`
	Notes      string      `json:"notes"`
	Background string      `json:"background"`
}

// UpdateSlideDeckRequest is the body for PATCH /slides/:id.
type UpdateSlideDeckRequest struct {
	Title      *string     `json:"title,omitempty"`
	Theme      *SlideTheme `json:"theme,omitempty"`
	Visibility *string     `json:"visibility,omitempty"`
}

// UpdateSlideRequest is the body for PATCH /slides/:id/slides/:slideID.
type UpdateSlideRequest struct {
	Layout     *SlideLayout `json:"layout,omitempty"`
	Title      *string      `json:"title,omitempty"`
	Body       *string      `json:"body,omitempty"`
	Bullets    *[]string    `json:"bullets,omitempty"`
	LeftCol    *string      `json:"left_col,omitempty"`
	RightCol   *string      `json:"right_col,omitempty"`
	ImageURL   *string      `json:"image_url,omitempty"`
	QuoteText  *string      `json:"quote_text,omitempty"`
	QuoteAttr  *string      `json:"quote_attr,omitempty"`
	Notes      *string      `json:"notes,omitempty"`
	Background *string      `json:"background,omitempty"`
}

// ListSlideDecksFilter narrows deck queries.
type ListSlideDecksFilter struct {
	KBID        string
	OwnerUserID uint64
	Visibility  string
	Limit       int
	Offset      int
}

// AutoGenerateRequest is the body for POST /slides/auto-generate.
type AutoGenerateRequest struct {
	SourceDocID string `json:"source_doc_id" binding:"required"`
	KBID        string `json:"kb_id"`
	Title       string `json:"title"`
	Theme       SlideTheme `json:"theme"`
	MaxSlides   int    `json:"max_slides"` // optional cap, default 20
}

// ExportFormat enumerates supported export targets.
type SlideExportFormat string

const (
	SlideExportFormatPPTX    SlideExportFormat = "pptx"
	SlideExportFormatHTML    SlideExportFormat = "html"
	SlideExportFormatPDF     SlideExportFormat = "pdf"
	SlideExportFormatJSON    SlideExportFormat = "json"
	SlideExportFormatMarkdown SlideExportFormat = "markdown"
)

// ValidSlideExportFormats is the closed set enforced at the API edge.
var ValidSlideExportFormats = map[SlideExportFormat]bool{
	SlideExportFormatPPTX:     true,
	SlideExportFormatHTML:     true,
	SlideExportFormatPDF:      true,
	SlideExportFormatJSON:     true,
	SlideExportFormatMarkdown: true,
}

// ErrSlideInvalid is a typed validation error.
type ErrSlideInvalid string

func (e ErrSlideInvalid) Error() string { return string(e) }

// IsSlideInvalid reports whether err is a validation failure.
func IsSlideInvalid(err error) bool {
	var t ErrSlideInvalid
	return errors.As(err, &t)
}
