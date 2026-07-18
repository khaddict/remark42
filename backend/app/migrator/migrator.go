// Package migrator provides import/export functionality. It defines Importer and Exporter interfaces
// and implements them for "native" remark (both importer and exporter).
// Also implements AutoBackup scheduler running exports as backups and saving them locally.
package migrator

import (
	"io"

	"github.com/umputun/remark42/backend/app/store"
	"github.com/umputun/remark42/backend/app/store/service"
)

// Importer defines interface to convert posts from external sources
type Importer interface {
	Import(r io.Reader, siteID string) (int, error)
}

// Exporter defines interface to export comments from internal store
type Exporter interface {
	Export(w io.Writer, siteID string) (int, error)
}

// Mapper defines interface to convert data in import procedure
type Mapper interface {
	URL(url string) string
}

// MapperMaker defines function that reads rules from reader and
// returns new Mapper with loaded rules. If rules are not valid
// it returns error.
type MapperMaker func(reader io.Reader) (Mapper, error)

// Store defines minimal interface needed to export and import comments
type Store interface {
	Create(comment store.Comment) (commentID string, err error)
	Find(locator store.Locator, sort string, user store.User) ([]store.Comment, error)
	List(siteID string, limit, skip int) ([]store.PostInfo, error)
	DeleteAll(siteID string) error
	Metas(siteID string) (umetas []service.UserMetaData, pmetas []service.PostMetaData, err error)
	SetMetas(siteID string, umetas []service.UserMetaData, pmetas []service.PostMetaData) error
}

var adminUser = store.User{Admin: true}
