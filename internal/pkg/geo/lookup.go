package geo

import (
	"net/netip"
)

type Info struct {
	City        string
	Country     string
	CountryCode string
	Region      string
	PostalCode  string
	// Latitude and Longitude are the city centroid, which is what makes an
	// implied-speed check possible. Zero when the lookup found nothing.
	Latitude  float64
	Longitude float64
	// AccuracyRadiusKm is MaxMind's own confidence in that point. A wide
	// radius means the coordinates are too vague to reason about distance.
	AccuracyRadiusKm uint16
}

// HasPosition reports whether the coordinates are usable. A zero pair is both
// "not found" and a real point in the Gulf of Guinea, so it is treated as
// missing rather than as a location.
func (i *Info) HasPosition() bool {
	return i != nil && (i.Latitude != 0 || i.Longitude != 0)
}

func (c *Client) Lookup(ip netip.Addr) (*Info, error) {
	info := &Info{}

	if ip.IsPrivate() || ip.IsLoopback() {
		info.City = "Local"
		info.Country = "Local Network"
		return info, nil
	}

	// No client, or no database yet, degrades to "Unknown" rather than
	// panicking — geo enrichment is best-effort and must never fail session
	// creation. The reader is held for the whole lookup so a refresh landing
	// mid-call cannot close the file underneath it.
	db, release := c.reader()
	defer release()
	if db != nil {
		cityRecord, err := db.City(ip)
		if err == nil && cityRecord != nil {
			info.City = cityRecord.City.Names.English
			if len(cityRecord.Subdivisions) > 0 {
				info.Region = cityRecord.Subdivisions[0].Names.English
			}
			info.Country = cityRecord.Country.Names.English
			info.CountryCode = cityRecord.Country.ISOCode
			info.PostalCode = cityRecord.Postal.Code
			// Nil coordinates mean the database has no position for this
			// address, which is different from a position at 0,0.
			if cityRecord.Location.Latitude != nil && cityRecord.Location.Longitude != nil {
				info.Latitude = *cityRecord.Location.Latitude
				info.Longitude = *cityRecord.Location.Longitude
			}
			info.AccuracyRadiusKm = cityRecord.Location.AccuracyRadius
		}
	}

	if info.City == "" {
		info.City = "Unknown"
	}

	if info.Country == "" {
		info.Country = "Unknown"
	}

	return info, nil
}
