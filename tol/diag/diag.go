package diag

import "fmt"

const (
	CodeParseUnexpected         = "TOL1001"
	CodeParseUnsupported        = "TOL1002"
	CodeSemaUnsupportedVer      = "TOL2001"
	CodeSemaMissingContract     = "TOL2002"
	CodeSemaDuplicateSlot       = "TOL2003"
	CodeSemaDuplicateFunction   = "TOL2004"
	CodeSemaCtorParams          = "TOL2005"
	CodeSemaBreakOutsideLoop    = "TOL2006"
	CodeSemaContinueOutsideLoop = "TOL2007"
	CodeSemaInvalidSetTarget    = "TOL2008"
	CodeSemaMissingCondition    = "TOL2009"
	CodeSemaInvalidSelector     = "TOL2010"
	CodeSemaDuplicateSelector   = "TOL2011"
	CodeSemaInvalidSelectorExpr = "TOL2012"
	CodeSemaSelectorTarget      = "TOL2013"
	CodeSemaInvalidFnModifier   = "TOL2014"
	CodeSemaConflictingModifier = "TOL2015"
	CodeSemaDuplicateParam      = "TOL2016"
	CodeSemaInvalidReturn       = "TOL2017"
	CodeSemaStorageAccess       = "TOL2018"
	CodeSemaCallArity           = "TOL2019"
	CodeSemaInvalidAssignExpr   = "TOL2020"
	CodeSemaInvalidStmtShape    = "TOL2021"
	CodeSemaInvalidRevert       = "TOL2022"
	CodeSemaEmitArity           = "TOL2023"
	CodeSemaDuplicateEvent      = "TOL2024"
	CodeSemaUnknownEmitEvent    = "TOL2025"
	CodeLowerNotImplemented     = "TOL3001"
	CodeLowerUnsupportedFeature = "TOL3002"
	CodeCodegenNotImplemented   = "TOL4001"
)

// Position describes a line/column position in a source file.
type Position struct {
	Line   int
	Column int
}

// Span describes a source range.
type Span struct {
	File  string
	Start Position
	End   Position
}

// Diagnostic is a structured compile-time error.
type Diagnostic struct {
	Code    string
	Message string
	Span    Span
}

func (d Diagnostic) Error() string {
	if d.Span.File == "" || d.Span.Start.Line <= 0 || d.Span.Start.Column <= 0 {
		return fmt.Sprintf("[%s] %s", d.Code, d.Message)
	}
	return fmt.Sprintf("%s:%d:%d: [%s] %s",
		d.Span.File,
		d.Span.Start.Line,
		d.Span.Start.Column,
		d.Code,
		d.Message,
	)
}

// Diagnostics is an ordered diagnostic list.
type Diagnostics []Diagnostic

func (ds Diagnostics) Error() string {
	if len(ds) == 0 {
		return ""
	}
	if len(ds) == 1 {
		return ds[0].Error()
	}
	return fmt.Sprintf("%s (and %d more error(s))", ds[0].Error(), len(ds)-1)
}

func (ds Diagnostics) HasErrors() bool { return len(ds) > 0 }
