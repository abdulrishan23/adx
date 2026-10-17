// Package providers holds one subpackage per outside API, such as Google Ads,
// Meta and GoHighLevel. Core never imports them. It declares the small
// interfaces it needs, and cmd/adx passes these clients in.
package providers
