package mmdb

import (
	"fmt"
	"net"

	"github.com/oschwald/maxminddb-golang"
	"github.com/sagernet/sing/common"
)

type geoip2Country struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type Reader struct {
	*maxminddb.Reader
	databaseType
}

func (r Reader) LookupCode(ipAddress net.IP) []string {
	switch r.databaseType {
	case typeMaxmind:
		var country geoip2Country
		_ = r.Lookup(ipAddress, &country)
		if country.Country.IsoCode == "" {
			return []string{}
		}
		return []string{country.Country.IsoCode}

	case typeSing:
		var code string
		_ = r.Lookup(ipAddress, &code)
		if code == "" {
			return []string{}
		}
		return []string{code}

	case typeMetaV0:
		var record any
		_ = r.Lookup(ipAddress, &record)
		switch record := record.(type) {
		case string:
			return []string{record}
		case []any: // lookup returned type of slice is []any
			return common.Map(record, func(it any) string {
				return it.(string)
			})
		}
		return []string{}

	default:
		panic(fmt.Sprint("unknown geoip database type:", r.databaseType))
	}
}
