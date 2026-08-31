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
