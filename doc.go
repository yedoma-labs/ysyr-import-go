// Package ysyrimport imports CSV data with partial success. Valid rows are returned, broken rows
// are reported with their line number, row number, column, code and a short English message.
// The report can be written back as CSV with WriteErrorReport.
//
// The package has no dependencies outside the standard library. It does not store anything:
// persisting the accepted rows, and any transaction around them, is up to the caller.
package ysyrimport
