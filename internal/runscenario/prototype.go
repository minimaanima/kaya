package runscenario

import (
	"kaya/internal/rungen"
	"kaya/internal/scenario"
)

func PrototypeDefinition() rungen.Definition {
	return rungen.Definition{
		ScenarioID:      scenario.PrototypeScenarioID,
		ScenarioVersion: scenario.PrototypeScenarioVersion,
		Build:           scenario.NewPrototypeTemplate,
		StartRoom:       scenario.RoomReception,
		WinRoom:         scenario.RoomStairwell,
		LightItem:       scenario.ItemFlashlight,
		ItemRules: []rungen.ItemRule{
			{
				ItemID: scenario.ItemFlashlight,
				Candidates: []rungen.PlacementCandidate{
					{ObjectID: scenario.ObjectReceptionDesk},
					{ObjectID: scenario.ObjectReceptionFloor},
					{ObjectID: scenario.ObjectCollapsedChair},
				},
			},
			{
				ItemID: scenario.ItemBrassKey,
				Candidates: []rungen.PlacementCandidate{
					{ObjectID: scenario.ObjectBodyCabinet},
					{ObjectID: scenario.ObjectBodyDoor},
					{ObjectID: scenario.ObjectStorageCabinet},
				},
			},
		},
	}
}
