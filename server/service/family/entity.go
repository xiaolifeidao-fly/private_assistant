package family

import (
	"time"

	"common/middleware/db"
)

type personRow struct {
	ID            int64
	PersonKey     string
	FamilyKey     string
	Name          string
	RoleLabel     string
	Aliases       db.StringList
	Gender        string
	Birthday      *time.Time
	LunarBirthday string
	BirthdayBy    string
	HomePlaceKey  string
	Side          string
	Importance    int
	IsCore        int
	Profile       string
	Avatar        string
	Sort          int
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (personRow) TableName() string { return "pa_person" }

func (row personRow) toDTO() Person {
	person := Person{
		PersonKey: row.PersonKey, Name: row.Name, RoleLabel: row.RoleLabel, Aliases: []string(row.Aliases),
		Gender: row.Gender, LunarBirthday: row.LunarBirthday, BirthdayBy: row.BirthdayBy, HomePlaceKey: row.HomePlaceKey,
		Side: row.Side, Importance: row.Importance, IsCore: row.IsCore == 1, Profile: row.Profile, Avatar: row.Avatar,
		Sort: row.Sort, Status: row.Status,
	}
	if person.Aliases == nil {
		person.Aliases = []string{}
	}
	if row.Birthday != nil {
		person.Birthday = row.Birthday.Format("2006-01-02")
	}
	return person
}

type relationRow struct {
	ID             int64
	RelationKey    string
	FamilyKey      string
	FromPerson     string
	ToPerson       string
	Relation       string
	AddressAs      string
	ChildAddressAs string
	Closeness      int
	Notes          string
	Sensitivities  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (relationRow) TableName() string { return "pa_person_relation" }

func (row relationRow) toDTO() Relation {
	return Relation{
		RelationKey: row.RelationKey, FromPerson: row.FromPerson, ToPerson: row.ToPerson, Relation: row.Relation,
		AddressAs: row.AddressAs, ChildAddressAs: row.ChildAddressAs, Closeness: row.Closeness,
		Notes: row.Notes, Sensitivities: row.Sensitivities,
	}
}

type placeRow struct {
	ID          int64
	PlaceKey    string
	FamilyKey   string
	Name        string
	Kind        string
	Address     string
	Dialect     string
	Description string
	Customs     string
	TravelTips  string
	Sort        int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (placeRow) TableName() string { return "pa_place" }

func (row placeRow) toDTO() Place {
	return Place{
		PlaceKey: row.PlaceKey, Name: row.Name, Kind: row.Kind, Address: row.Address, Dialect: row.Dialect,
		Description: row.Description, Customs: row.Customs, TravelTips: row.TravelTips, Sort: row.Sort,
	}
}
