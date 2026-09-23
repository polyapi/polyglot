// Package glide is the Glide v2 pipeline: find deployables, validate, plan, push, pull.
//
// The Go host walks the repo (JSONC parse plus a small language table: extensions,
// comment syntax, a polyConfig text marker). Language adapters transpile/eval code
// modules (extract) and rewrite docs (prepare). There is no lazy/cached-deployable
// mode. Deploy receipts are optional.
package glide
