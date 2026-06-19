package handler

import "github.com/noxaaa/prism-oss/pkg/edition"

func defaultControlServerEdition() edition.Provider {
	return edition.OSSProvider()
}
