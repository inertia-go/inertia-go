package inertia

import "errors"

// Sentinel errors the caller may branch on via errors.Is.
var (
	ErrSessionRequired      = errors.New("inertia: session store is required when using errors or flash")
	ErrTemplateNotFound     = errors.New("inertia: root template not found")
	ErrCookieTooLarge       = errors.New("inertia: cookie payload exceeds 3.5KB")
	ErrSSRUnavailable       = errors.New("inertia: ssr render failed and SSRRequired=true")
	ErrPropEvaluationFailed = errors.New("inertia: prop evaluation failed")
	ErrConflictingVersion   = errors.New("inertia: Config has more than one Version source set")
)
