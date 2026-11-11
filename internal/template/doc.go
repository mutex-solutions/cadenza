// Package template defines Cadenza templates: a named list of slots, each
// filled with one block (text, image, table, ...), plus a guide and an
// example that teach agents how to fill it (plan §4, D7).
//
// The same checks run twice. The CLI validates data in Local mode, where
// files are local paths, uploads those files, swaps the paths for stored
// references with ReplacePaths, and sends the result. The server validates
// it again in Remote mode, where every file must be a media.FileRef of the
// right type, and then calls Render to make the view data the apps show.
//
// The package is pure: it does no I/O apart from reading the embedded
// built-in templates.
package template
