package handler_test

import (
	"github.com/tus/tusd/v3/pkg/filestore"
	"github.com/tus/tusd/v3/pkg/handler"
	"github.com/tus/tusd/v3/pkg/memorylocker"
)

func ExampleNewStoreComposer() {
	composer := handler.NewStoreComposer()

	fs := filestore.New("./data")
	fs.DirModePerm = 0775
	fs.FileModePerm = 0664
	fs.UseIn(composer)

	ml := memorylocker.New()
	ml.UseIn(composer)

	config := handler.Config{
		StoreComposer: composer,
	}

	_, _ = handler.NewHandler(config)
}
