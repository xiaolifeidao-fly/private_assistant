package reminder

import (
	"context"

	"service/family"
	"service/memory"
)

type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
	ListPlaces(ctx context.Context, familyKey string) ([]family.Place, error)
	ListRelations(ctx context.Context, familyKey, personKey string) ([]family.Relation, error)
	GetPlace(ctx context.Context, familyKey, placeKey string) (family.Place, error)
}

type MemoryPort interface {
	List(ctx context.Context, familyKey string, filter memory.ListFilter) ([]memory.Memory, int64, error)
}
