package interfaces

import "errors"

// Wiki comment sentinels. Defined alongside the wiki tag sentinels in
// the same interfaces package so the handler can errors.Is() check
// without importing each service module.
var (
	ErrWikiCommentNotFound  = errors.New("wiki comment not found")
	ErrWikiCommentConflict  = errors.New("wiki comment conflict")
	ErrWikiCommentForbidden = errors.New("wiki comment forbidden")
	ErrWikiCommentBadInput  = errors.New("wiki comment bad input")
)

// Inline AI sentinels. The service translates LLM failures into
// ErrInlineAIDown so the handler can render a friendly "AI temporarily
// unavailable" toast without leaking provider internals.
var (
	ErrInlineAIUnavailable = errors.New("inline ai: model unavailable")
)

// Audit export sentinels. The service translates "row not found"
// into ErrAuditExportNotFound so the handler maps to 404 cleanly.
var (
	ErrAuditExportNotFound = errors.New("audit export not found")
	ErrAuditExportForbidden = errors.New("audit export forbidden")
)
