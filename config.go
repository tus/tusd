package tusd

import (
	"log"
	"net/url"
	"os"
)

// Config provides a way to configure the Handler depending on your needs.
type Config struct {
	// DataStore implementation used to store and retrieve the single uploads.
	// Must no be nil.
	DataStore     DataStore
	StoreComposer *StoreComposer
	// MaxSize defines how many bytes may be stored in one single upload. If its
	// value is is 0 or smaller no limit will be enforced.
	MaxSize int64
	// BasePath defines the URL path used for handling uploads, e.g. "/files/".
	// If no trailing slash is presented it will be added. You may specify an
	// absolute URL containing a scheme, e.g. "http://tus.io"
	BasePath string
	isAbs    bool
	// Initiate the CompleteUploads channel in the Handler struct in order to
	// be notified about complete uploads
	NotifyCompleteUploads bool
	// Logger the logger to use internally
	Logger *log.Logger
	// Respect the X-Forwarded-Host, X-Forwarded-Proto and Forwarded headers
	// potentially set by proxies when generating an absolute URL in the
	// reponse to POST requests.
	RespectForwardedHeaders bool
}

func (config *Config) validate() error {
	if config.Logger == nil {
		config.Logger = log.New(os.Stdout, "[tusd] ", 0)
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
		config.StoreComposer = NewStoreComposerFromDataStore(config.DataStore)
	} else if config.DataStore != nil {
		// TODO: consider returning an error
	}

	return nil
}
