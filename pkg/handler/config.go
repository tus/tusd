package handler

import (
	"errors"
	"log"
	"net/url"
	"os"
)

// Config provides a way to configure the Handler depending on your needs.
type Config struct {
	// StoreComposer points to the store composer from which the core data store
	// and optional dependencies should be taken. May only be nil if DataStore is
	// set.
	// TODO: Remove pointer?
	StoreComposer *StoreComposer
	// MaxSize defines how many bytes may be stored in one single upload. If its
	// value is is 0 or smaller no limit will be enforced.
	MaxSize int64
	// BasePath defines the URL path used for handling uploads, e.g. "/files/".
	// If no trailing slash is presented it will be added. You may specify an
	// absolute URL containing a scheme, e.g. "http://tus.io"
	BasePath string
	isAbs    bool
	// EnableExperimentalProtocol controls whether the new resumable upload protocol draft
	// from the IETF's HTTP working group is accepted next to the current tus v1 protocol.
	// See https://datatracker.ietf.org/doc/draft-ietf-httpbis-resumable-upload/
	EnableExperimentalProtocol bool
	// DisableDownload indicates whether the server will refuse downloads of the
	// uploaded file, by not mounting the GET handler.
	DisableDownload bool
	// DisableTermination indicates whether the server will refuse termination
	// requests of the uploaded file, by not mounting the DELETE handler.
	DisableTermination bool
	// Disable cors headers. If set to true, tusd will not send any CORS related header.
	// This is useful if you have a proxy sitting in front of tusd that handles CORS.
	DisableCors bool
	// NotifyCompleteUploads indicates whether sending notifications about
	// completed uploads using the CompleteUploads channel should be enabled.
	NotifyCompleteUploads bool
	// NotifyTerminatedUploads indicates whether sending notifications about
	// terminated uploads using the TerminatedUploads channel should be enabled.
	NotifyTerminatedUploads bool
	// NotifyUploadProgress indicates whether sending notifications about
	// the upload progress using the UploadProgress channel should be enabled.
	NotifyUploadProgress bool
	// NotifyCreatedUploads indicates whether sending notifications about
	// the upload having been created using the CreatedUploads channel should be enabled.
	NotifyCreatedUploads bool
	// Logger is the logger to use internally, mostly for printing requests.
	Logger *log.Logger
	// Explicitly set Access-Control-Allow-Origin in cases where RespectForwardedHeaders
	// doesn't give you the desired result. This can be the case with some reverse proxies
	// or a kubernetes setup with complex network routing rules
	CorsOrigin string
	// Respect the X-Forwarded-Host, X-Forwarded-Proto and Forwarded headers
	// potentially set by proxies when generating an absolute URL in the
	// response to POST requests.
	RespectForwardedHeaders bool
	// PreUploadCreateCallback will be invoked before a new upload is created, if the
	// property is supplied. If the callback returns nil, the upload will be created.
	// Otherwise the HTTP request will be aborted. This can be used to implement
	// validation of upload metadata etc.
	PreUploadCreateCallback func(hook HookEvent) error
	// PreFinishResponseCallback will be invoked after an upload is completed but before
	// a response is returned to the client. Error responses from the callback will be passed
	// back to the client. This can be used to implement post-processing validation.
	PreFinishResponseCallback func(hook HookEvent) error
}

func (config *Config) validate() error {
	if config.Logger == nil {
		config.Logger = log.New(os.Stdout, "[tusd] ", log.Ldate|log.Lmicroseconds)
	}

	base := config.BasePath
	uri, err := url.Parse(base)
	if err != nil {
		return err
	}

	// Ensure base path ends with slash to remove logic from absFileURL
	if base != "" && string(base[len(base)-1]) != "/" {
		base += "/"
	}

	// Ensure base path begins with slash if not absolute (starts with scheme)
	if !uri.IsAbs() && len(base) > 0 && string(base[0]) != "/" {
		base = "/" + base
	}
	config.BasePath = base
	config.isAbs = uri.IsAbs()

	if config.StoreComposer == nil {
		return errors.New("tusd: StoreComposer must no be nil")
	}

	if config.StoreComposer.Core == nil {
		return errors.New("tusd: StoreComposer in Config needs to contain a non-nil core")
	}

	if config.CorsOrigin != "" && config.CorsOrigin != "*" && config.CorsOrigin != "null" {
		_, err := url.ParseRequestURI(config.CorsOrigin)
		if err != nil {
			return errors.New("tusd: CorsOrigin is not a valid URL")
		}
	}

	return nil
}
