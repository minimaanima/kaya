package game

type ActionContext struct {
	CurrentRoom  RoomID
	TargetObject ObjectID
	TargetDoor   DoorID
	Item         ItemID
	HasLight     bool
	NearbyDoors  []DoorID
	NowSeconds   int
}
