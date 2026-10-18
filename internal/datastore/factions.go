package datastore

import (
	"database/sql"
	"encoding/json"
)

// StoreFactions upserts factions for the current reset (one fetch per
// reset). Traits are marshaled to JSON in the traits column; the Reset
// field is stamped from currentReset.
func StoreFactions(fac []Faction) error {
	return inTx(func(tx *sql.Tx) error {
		ins, err := tx.Prepare(`INSERT OR REPLACE INTO factions
			(reset_date, symbol, name, description, headquarters, traits, is_recruiting)
			VALUES (?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for _, f := range fac {
			traits, err := json.Marshal(f.Traits)
			if err != nil {
				return err
			}
			recruiting := 0
			if f.IsRecruiting {
				recruiting = 1
			}
			if _, err := ins.Exec(string(currentReset), f.Symbol, f.Name, f.Description,
				f.Headquarters, string(traits), recruiting); err != nil {
				return err
			}
		}
		return nil
	})
}

func GetFactions(reset Reset) ([]Faction, error) {
	rows, err := db.Query(`SELECT reset_date, symbol, name, description, headquarters, traits, is_recruiting
		FROM factions WHERE reset_date = ? ORDER BY symbol`, resetOrCurrent(reset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []Faction{}
	for rows.Next() {
		var f Faction
		var traits string
		var recruiting int
		if err := rows.Scan(&f.Reset, &f.Symbol, &f.Name, &f.Description, &f.Headquarters, &traits, &recruiting); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(traits), &f.Traits); err != nil {
			return nil, err
		}
		f.IsRecruiting = recruiting != 0
		res = append(res, f)
	}
	return res, rows.Err()
}
