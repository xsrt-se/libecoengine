package main

import "C"

import (
	cl "eco-engine/customlog"
	"eco-engine/table"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"time"

	"github.com/RyanCarrier/dijkstra"
	"github.com/gookit/goutil/arrutil"
)

var (
	t                 map[string]*table.Territory // loaded
	loadedTerritories = make(map[string]*table.Territory)
	upgrades          *table.CostTable
	initialised       = false
	hq                string
)

func main() {

}

func startTimer(t *map[string]*table.Territory, HQ string) {

	// run generateResource every 1s and resTick every 60s using goroutine
	go func(t *map[string]*table.Territory, HQ string) {
		// log.Println("New goroutine started")
		var counter = 0
		for {
			time.Sleep(time.Second * 1)
			GenerateResorce(t)
			CalculateTowerStats(t, HQ)
			CalculateTerritoryUsageCost(t)
			CalculateTerritoryLevel(t)
			SetStorageCapacity(t)

			counter++
			cl.Debug("Tick", counter)
			cl.Debug("HQ", (*t)[HQ].Property, (*t)[HQ].Storage.Current)
			cl.Debug("Ahmsord", (*t)["Ahmsord"].TerritoryUsage, (*t)["Ahmsord"].Storage.Current)
			// every 60s
			if counter%5 == 0 {
				ResourceTick(t, HQ)
				ResourceTickFromHQ(t, HQ)
				RequestResourceFromHQ(t)
				cl.Debug("Tock")
				counter = 0
			}
		}
	}(t, HQ)
}

func CalculateTowerStats(t *map[string]*table.Territory, HQ string) {
	for _, territory := range *t {

		// if not claimed then it will be default value
		if (*territory).Claim {

			// calculate tower stats based on the upgrade value and nearby territories

			if (*territory).Property.HQ {

				var conns, ext = 0, 0

				for _, terr := range *t {
					if len((*terr).RouteToHQ) <= 3 && (*terr).Claim {
						ext++
					}
				}
				for _, terr := range (*t)[HQ].TradingRoutes {
					if (*t)[terr].Claim {
						conns++
					}
				}

				var tr = (*territory)

				// 50% damage boost for hq
				var towerDmgLevel = tr.Property.TargetUpgrades.Damage
				var towerAtkLevel = tr.Property.TargetUpgrades.Attack
				var towerDefLevel = tr.Property.TargetUpgrades.Defence
				var towerHpLevel = tr.Property.TargetUpgrades.Health

				var baseDamageMin = upgrades.UpgradeBaseStats.Damage.Min[towerDmgLevel]
				var baseDamageMax = upgrades.UpgradeBaseStats.Damage.Max[towerDmgLevel]
				var baseAttack = upgrades.UpgradeBaseStats.Attack[towerAtkLevel]
				var baseHp = upgrades.UpgradeBaseStats.Health[towerHpLevel]
				var baseDefence = upgrades.UpgradeBaseStats.Defence[towerDefLevel]

				// hq conns and ext buff
				var dmgMin float64 = float64(baseDamageMin) * (1.5) * (1 + (0.3 * float64(conns))) * (1 + (0.25 * float64(ext)))
				var dmgMax float64 = float64(baseDamageMax) * (1.5) * (1 + (0.3 * float64(conns))) * (1 + (0.25 * float64(ext)))
				// hp
				var hp float64 = float64(baseHp) * (1 + (0.3 * float64(conns))) * (1 + (0.25 * float64(ext)))

				tr.Stats.Damage.Min = uint64(math.Round(dmgMin))
				tr.Stats.Damage.Max = uint64(math.Round(dmgMax))
				tr.Stats.Attack = float32(baseAttack)
				tr.Stats.Health = uint64(hp)
				tr.Stats.Defence = float32(baseDefence)

			}

		} else if !(*territory).Property.HQ && (*territory).Claim {

			var conns = 0

			for _, terr := range (*territory).TradingRoutes {
				if (*t)[terr].Claim {
					conns++
				}
			}

			var tr = (*territory)

			// conns gives 30% damage and hp boost to normal terrs
			var towerDmgLevel = tr.Property.CurrentUpgrades.Damage
			var towerAtkLevel = tr.Property.CurrentUpgrades.Attack
			var towerDefLevel = tr.Property.CurrentUpgrades.Defence
			var towerHpLevel = tr.Property.CurrentUpgrades.Health

			var baseDamageMin = upgrades.UpgradeBaseStats.Damage.Min[towerDmgLevel]
			var baseDamageMax = upgrades.UpgradeBaseStats.Damage.Max[towerDmgLevel]
			var baseAttack = upgrades.UpgradeBaseStats.Attack[towerAtkLevel]
			var baseHp = upgrades.UpgradeBaseStats.Health[towerHpLevel]
			var baseDefence = upgrades.UpgradeBaseStats.Defence[towerDefLevel]

			var dmgMin float64 = float64(baseDamageMin) * (1.5) * (1 + (0.3 * float64(conns)))
			var dmgMax float64 = float64(baseDamageMax) * (1.5) * (1 + (0.3 * float64(conns)))

			tr.Stats.Damage.Min = uint64(math.Round(dmgMin))
			tr.Stats.Damage.Max = uint64(math.Round(dmgMax))
			tr.Stats.Attack = float32(baseAttack)
			tr.Stats.Health = uint64(math.Round(float64(baseHp) * (1 + (0.3 * float64(conns)))))
			tr.Stats.Defence = float32(baseDefence)

		}

		(*territory).Stats.Damage.Min = 1_000
		(*territory).Stats.Damage.Max = 1_500
		(*territory).Stats.Attack = 0.5
		(*territory).Stats.Health = 300_000
		(*territory).Stats.Defence = 10

		(*territory).Stats.StrongerMinions = 0
		(*territory).Stats.TowerMultiAttacks = 0
		(*territory).Stats.TowerAura = 0
		(*territory).Stats.TowerVolley = 0
	}

	UseResource(t)
}

func UseResource(t *map[string]*table.Territory) {

	cl.Debug("Called")
	for _, territory := range *t {

		// offload the calculation to another goroutine
		go func(t *table.Territory) {
			// log.Println("New goroutine started for terriotry: ", (*t).Name)

			var towerDmgLevel = (*t).Property.TargetUpgrades.Damage
			var towerAtkLevel = (*t).Property.TargetUpgrades.Attack
			var towerDefLevel = (*t).Property.TargetUpgrades.Defence
			var towerHpLevel = (*t).Property.TargetUpgrades.Health

			var strongerMinionsLevel = (*t).Property.TargetBonuses.StrongerMinions
			var towerMultiAttackLevel = (*t).Property.TargetBonuses.TowerMultiAttack
			var towerAuraLevel = (*t).Property.TargetBonuses.TowerAura
			var towerVolleyLevel = (*t).Property.TargetBonuses.TowerVolley
			var largerEmeraldsStorageLevel = (*t).Property.TargetBonuses.LargerEmeraldStorage
			var largerResourceStorageLevel = (*t).Property.TargetBonuses.LargerResourceStorage
			var efficientResourceLevel = (*t).Property.TargetBonuses.EfficientResource
			var efficientEmeraldLevel = (*t).Property.TargetBonuses.EfficientEmerald
			var resourceRateLevel = (*t).Property.TargetBonuses.ResourceRate
			var emeraldRateLevel = (*t).Property.TargetBonuses.EmeraldRate

			var damageCost = upgrades.UpgradesCost.Damage.Value[towerDmgLevel]
			var attackCost = upgrades.UpgradesCost.Attack.Value[towerAtkLevel]
			var defenceCost = upgrades.UpgradesCost.Defence.Value[towerDefLevel]
			var healthCost = upgrades.UpgradesCost.Health.Value[towerHpLevel]

			var strongerMinionsCost = upgrades.Bonuses.StrongerMinions.Cost[strongerMinionsLevel]
			var towerMultiAttackCost = upgrades.Bonuses.TowerMultiAttack.Cost[towerMultiAttackLevel]
			var towerAuraCost = upgrades.Bonuses.TowerAura.Cost[towerAuraLevel]
			var towerVolleyCost = upgrades.Bonuses.TowerVolley.Cost[towerVolleyLevel]

			var largerEmeraldsStorageCost = upgrades.Bonuses.LargerEmeraldsStorage.Cost[largerEmeraldsStorageLevel]
			var largerResourceStorageCost = upgrades.Bonuses.LargerResourceStorage.Cost[largerResourceStorageLevel]
			var efficientResourceCost = upgrades.Bonuses.EfficientResource.Cost[efficientResourceLevel]
			var efficientEmeraldCost = upgrades.Bonuses.EfficientEmeralds.Cost[efficientEmeraldLevel]
			var resourceRateCost = upgrades.Bonuses.ResourceRate.Cost[resourceRateLevel]
			var emeraldRateCost = upgrades.Bonuses.EmeraldsRate.Cost[emeraldRateLevel]

			var emeraldCostPerSec = float64(largerResourceStorageCost+efficientResourceCost+resourceRateCost) / 3600 // 1 hour
			var oreCostPerSec = float64(damageCost+towerVolleyCost+efficientEmeraldCost) / 3600
			var cropCostPerSec = float64(attackCost+towerAuraCost+emeraldRateCost) / 3600
			var woodCostPerSec = float64(healthCost+strongerMinionsCost+largerEmeraldsStorageCost) / 3600
			var fishCostPerSec = float64(defenceCost+towerMultiAttackCost) / 3600

			if (*t).Storage.Current.Emerald < emeraldCostPerSec {
				(*t).Property.CurrentBonuses.LargerResourceStorage = 0
				(*t).Property.CurrentBonuses.EfficientResource = 0
				(*t).Property.CurrentBonuses.ResourceRate = 0
			} else if (*t).Storage.Current.Ore >= oreCostPerSec {
				(*t).Property.CurrentBonuses.LargerResourceStorage = (*t).Property.TargetBonuses.LargerResourceStorage
				(*t).Property.CurrentBonuses.EfficientResource = (*t).Property.TargetBonuses.EfficientResource
				(*t).Property.CurrentBonuses.ResourceRate = (*t).Property.TargetBonuses.ResourceRate
			}

			if (*t).Storage.Current.Ore < oreCostPerSec {
				(*t).Property.CurrentUpgrades.Damage = 0
				(*t).Property.CurrentBonuses.TowerVolley = 0
				(*t).Property.CurrentBonuses.EfficientEmerald = 0
			} else if (*t).Storage.Current.Crop >= cropCostPerSec {
				(*t).Property.CurrentUpgrades.Damage = (*t).Property.TargetUpgrades.Damage
				(*t).Property.CurrentBonuses.TowerVolley = (*t).Property.TargetBonuses.TowerVolley
				(*t).Property.CurrentBonuses.EfficientEmerald = (*t).Property.TargetBonuses.EfficientEmerald
			}

			if (*t).Storage.Current.Crop < cropCostPerSec {
				(*t).Property.CurrentUpgrades.Attack = 0
				(*t).Property.CurrentBonuses.TowerAura = 0
				(*t).Property.CurrentBonuses.EmeraldRate = 0
			} else if (*t).Storage.Current.Crop >= cropCostPerSec {
				(*t).Property.CurrentUpgrades.Attack = (*t).Property.TargetUpgrades.Attack
				(*t).Property.CurrentBonuses.TowerAura = (*t).Property.TargetBonuses.TowerAura
				(*t).Property.CurrentBonuses.EmeraldRate = (*t).Property.TargetBonuses.EmeraldRate
			}

			if (*t).Storage.Current.Wood < woodCostPerSec {
				(*t).Property.CurrentUpgrades.Health = 0
				(*t).Property.CurrentBonuses.StrongerMinions = 0
				(*t).Property.CurrentBonuses.LargerEmeraldStorage = 0
			} else if (*t).Storage.Current.Wood >= woodCostPerSec {
				(*t).Property.CurrentUpgrades.Health = (*t).Property.TargetUpgrades.Health
				(*t).Property.CurrentBonuses.StrongerMinions = (*t).Property.TargetBonuses.StrongerMinions
				(*t).Property.CurrentBonuses.LargerEmeraldStorage = (*t).Property.TargetBonuses.LargerEmeraldStorage
			}

			if (*t).Storage.Current.Fish < fishCostPerSec {
				(*t).Property.CurrentUpgrades.Defence = 0
				(*t).Property.CurrentBonuses.TowerMultiAttack = 0
			} else if (*t).Storage.Current.Fish >= fishCostPerSec {
				(*t).Property.CurrentUpgrades.Defence = (*t).Property.TargetUpgrades.Defence
				(*t).Property.CurrentBonuses.TowerMultiAttack = (*t).Property.TargetBonuses.TowerMultiAttack
			}

		}(territory)
	}
}

func SetStorageCapacity(territories *map[string]*table.Territory) {
	for _, territory := range *territories {
		var largerResourceStorageLevel = (*territory).Property.CurrentBonuses.LargerResourceStorage
		var largerEmeraldsStorageLevel = (*territory).Property.CurrentBonuses.LargerEmeraldStorage

		if !(*territory).Property.HQ {
			(*territory).Storage.Capacity.Emerald = float64(3000 * upgrades.Bonuses.LargerEmeraldsStorage.Value[largerEmeraldsStorageLevel])
			(*territory).Storage.Capacity.Ore = float64(300 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Crop = float64(300 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Wood = float64(300 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Fish = float64(300 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
		} else {
			(*territory).Storage.Capacity.Emerald = float64(5000 * upgrades.Bonuses.LargerEmeraldsStorage.Value[largerEmeraldsStorageLevel])
			(*territory).Storage.Capacity.Ore = float64(1500 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Crop = float64(1500 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Wood = float64(1500 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
			(*territory).Storage.Capacity.Fish = float64(1500 * upgrades.Bonuses.LargerResourceStorage.Value[largerResourceStorageLevel])
		}
	}
}

func GenerateResorce(territories *map[string]*table.Territory) {
	// rate means how many seconds it takes to generate n resource
	// n resource is calculated like this
	// nres = base res prod * efficient resource
	// so if rate is level 0 then it takes 1 second to generate 1/4 of the resource
	// and if resource stored in the storage excees the capacity then the excess resource will be lost
	// stoarge capacity is calculated like this
	// cap = base cap * larger storage

	for _, territory := range *territories {

		// emerald generation
		var baseEmeraldGeneration = (*territory).BaseResourceProduction.Emerald
		var efficientEmerald = (*territory).Property.TargetBonuses.EfficientEmerald
		var emeraldRate = (*territory).Property.TargetBonuses.EmeraldRate

		var emeraldMultiplier = float64(upgrades.Bonuses.EfficientEmeralds.Value[efficientEmerald]) * (4 / float64(upgrades.Bonuses.EmeraldsRate.Value[emeraldRate]))

		var emeraldGenerationPerSec = (float64(baseEmeraldGeneration) * emeraldMultiplier) / 3600

		// add the emerald to storage
		if (*territory).Storage.Current.Emerald < (*territory).Storage.Capacity.Emerald {
			(*territory).Storage.Current.Emerald += emeraldGenerationPerSec
		} else {
			(*territory).Storage.Current.Emerald = (*territory).Storage.Capacity.Emerald
		}

		// resource generation
		// check what kind of territory is this first
		var territoryType string

		if (*territory).BaseResourceProduction.Crop != 0 {
			territoryType = "Crop"
		} else if (*territory).BaseResourceProduction.Wood != 0 {
			territoryType = "Wood"
		} else if (*territory).BaseResourceProduction.Ore != 0 {
			territoryType = "Ore"
		} else if (*territory).BaseResourceProduction.Fish != 0 {
			territoryType = "Fish"
		} else if (*territory).BaseResourceProduction.Fish != 0 && (*territory).BaseResourceProduction.Crop != 0 {

			// gotta accomodate for that one stupid terr in ragni area
			territoryType = "FishCrop"
		}

		if territory.Name != "Maltic Coast" {

			// get struct field by string
			var baseResourceGeneration = reflect.ValueOf((*territory).BaseResourceProduction).FieldByName(territoryType).Float()
			var efficientResource = (*territory).Property.CurrentBonuses.EfficientResource
			var resourceRate = (*territory).Property.CurrentBonuses.ResourceRate

			var resourceMultiplier = float64(upgrades.Bonuses.EfficientResource.Value[efficientResource]) * (4 / float64(upgrades.Bonuses.ResourceRate.Value[resourceRate]))
			var resourceGenerationPerSec = (float64(baseResourceGeneration) * resourceMultiplier) / 3600

			// add the resource to storage using runtime reflection since we dont know what kind of resource it is
			var resourceStorage = reflect.ValueOf((*territory).Storage.Current).FieldByName(territoryType).Float()
			var resourceStorageCapacity = reflect.ValueOf((*territory).Storage.Capacity).FieldByName(territoryType).Float()

			// use reflection to set the value of the field
			var v = reflect.ValueOf(&territory.Storage.Current).Elem()
			var f = v.FieldByName(territoryType)

			// check if the field is valid and can be set
			if f.IsValid() && f.CanSet() {
				if resourceStorage < resourceStorageCapacity {
					f.SetFloat(resourceStorage + float64(resourceGenerationPerSec))
				} else {
					f.SetFloat(resourceStorageCapacity)
				}
			} else {
				cl.Error("Field is not valid or cannot be set")
			}
		} else {

			// for the maltic plains
			var baseResourceGenerationCrop = (*territory).BaseResourceProduction.Crop
			var baseResourceGenerationFish = (*territory).BaseResourceProduction.Fish

			var efficientResource = (*territory).Property.CurrentBonuses.EfficientResource
			var resourceRate = (*territory).Property.CurrentBonuses.ResourceRate

			var resourceMultiplier = float64(upgrades.Bonuses.EfficientResource.Value[efficientResource]) * (4 / float64(upgrades.Bonuses.ResourceRate.Value[resourceRate]))

			var resourceGenerationPerSecCrop = (float64(baseResourceGenerationCrop) * resourceMultiplier) / 3600
			var resourceGenerationPerSecFish = (float64(baseResourceGenerationFish) * resourceMultiplier) / 3600

			// for crops
			if (*territory).Storage.Current.Crop <= (*territory).Storage.Capacity.Crop {
				(*territory).Storage.Current.Crop += resourceGenerationPerSecCrop
			} else {
				(*territory).Storage.Current.Crop = (*territory).Storage.Capacity.Crop
			}

			// for fish
			if (*territory).Storage.Current.Fish <= (*territory).Storage.Capacity.Fish {
				(*territory).Storage.Current.Fish += resourceGenerationPerSecFish
			} else {
				(*territory).Storage.Current.Fish = (*territory).Storage.Capacity.Fish
			}
		}
	}
}

func CalculateTerritoryUsageCost(territories *map[string]*table.Territory) {
	for _, territory := range *territories {

		//ignore if the territory is not claimed
		if !territory.Claim {
			continue
		}

		var usage = table.TerritoryResource{
			Emerald: 0,
			Ore:     0,
			Wood:    0,
			Crop:    0,
			Fish:    0,
		}

		var damage, attack, hp, defence int
		damage = territory.Property.TargetUpgrades.Damage
		attack = territory.Property.TargetUpgrades.Attack
		hp = territory.Property.TargetUpgrades.Health
		defence = territory.Property.TargetUpgrades.Defence

		// calculate the upgrade usage cost of the territory
		usage.Ore += float64(upgrades.UpgradesCost.Damage.Value[damage])
		usage.Crop += float64(upgrades.UpgradesCost.Attack.Value[attack])
		usage.Wood += float64(upgrades.UpgradesCost.Health.Value[hp])
		usage.Fish += float64(upgrades.UpgradesCost.Defence.Value[defence])

		// bonuses
		var strongerMinions = territory.Property.TargetBonuses.StrongerMinions
		var towerMultiAttacks = territory.Property.TargetBonuses.TowerMultiAttack
		var aura = territory.Property.TargetBonuses.TowerAura
		var volley = territory.Property.TargetBonuses.TowerVolley

		var efficientResource = territory.Property.TargetBonuses.EfficientResource
		var resourceRate = territory.Property.TargetBonuses.ResourceRate
		var efficientEmerald = territory.Property.TargetBonuses.EfficientEmerald
		var emeraldRate = territory.Property.TargetBonuses.EmeraldRate

		var emStorage = territory.Property.TargetBonuses.LargerEmeraldStorage
		var resStorage = territory.Property.TargetBonuses.LargerResourceStorage

		usage.Ore += float64(upgrades.Bonuses.StrongerMinions.Cost[volley] + upgrades.Bonuses.EfficientEmeralds.Cost[efficientEmerald])
		usage.Crop += float64(upgrades.Bonuses.TowerAura.Cost[aura] + upgrades.Bonuses.EmeraldsRate.Cost[emeraldRate])
		usage.Wood += float64(upgrades.Bonuses.StrongerMinions.Cost[strongerMinions] + upgrades.Bonuses.LargerEmeraldsStorage.Cost[emStorage] + upgrades.Bonuses.StrongerMinions.Cost[strongerMinions])
		usage.Fish += float64(upgrades.Bonuses.TowerMultiAttack.Cost[towerMultiAttacks])
		usage.Emerald += float64(upgrades.Bonuses.LargerResourceStorage.Cost[resStorage] + upgrades.Bonuses.ResourceRate.Cost[resourceRate] + upgrades.Bonuses.EfficientResource.Cost[efficientResource])

		// set the usage cost of the territory
		(*territory).TerritoryUsage = usage
		cl.Debug("Usage for", (territory.Name), (*territory).TerritoryUsage)
	}
}

func RequestResourceFromHQ(territories *map[string]*table.Territory) {

}

func ResourceTickFromHQ(t *map[string]*table.Territory, HQ string) {

}

func ResourceTick(territories *map[string]*table.Territory, HQ string) {

}

func GetPathToHQCheapest(territories *map[string]*table.Territory, HQ string) {

	// name is the HQ territory name
	// get path to hq using dijkstra, depending on the trading style
	// fastest  int
	// will find the shortest path while cheapest will find the shortest path with the least GLOBAL tax
	// if the territory is the hq then return empty array

	// connected nodes (territory) can be found at territories[name].TradingRoutes
	var graph = dijkstra.NewGraph()
	var HQID int

	// find the id of hq
	for _, territory := range *territories {
		if territory.Property.HQ {
			HQID = territory.ID
			break
		}
	}

	var vertexAdded = make(map[int]bool)

	for name := range *territories {
		// Add logic to compute the shortest path to HQ using Dijkstra's algorithm
		// add current node
		if vertexAdded[(*territories)[name].ID] {
			cl.Warn("Vertex ID :", (*territories)[name].ID, "already added")
			continue
		} else {
			graph.AddVertex((*territories)[name].ID)
			// log.Println("Added vertex ID:", territories[name].ID)
		}
	}

	// now add arc
	for _, territory := range *territories {

		var currTerr = territory.ID
		// log.Println("Current territory ID:", currTerr, " ", territory.Name)
		for _, route := range territory.TradingRoutes {

			var currConn = (*territories)[route].ID

			// log.Println("Connection ID", currConn, " ", route)

			// distance is the tax value
			var distance = float64((*territories)[route].Property.Tax.Others)
			var err = graph.AddArc(currTerr, currConn, int64(distance))
			if err != nil {
				cl.Error("Error adding arc :", err)
			}

		}
	}

	// get terr id
	for _, territory := range *territories {
		if territory.Property.HQ {
			continue
		}

		var terrID = territory.ID
		// cl.Log(territory.Name, terrID, "\n", HQ, HQID)
		var pathToHQRaw, err = graph.ShortestSafe(terrID, HQID)
		if err != nil {
			cl.Warn("Error in territory :", territory.Name)
			cl.Warn(territory.TradingRoutes)
			cl.Error("Error getting shortest path to HQ :", err)
		}

		// Assign path to HQ to the territory
		// Convert terr ID to terr name and store in pathToHQ
		var pathList = pathToHQRaw.Path
		var path = make([]string, len(pathList))
		for i, id := range pathList {
			for _, terr := range *territories {
				if terr.ID == id {
					path[i] = terr.Name
					break
				}
			}
		}

		(*territory).RouteToHQ = path
		var reversePath = make([]string, len(path))
		copy(reversePath, path)
		arrutil.Reverse(reversePath)
		(*territory).RouteFromHQ = reversePath
	}
}

func GetPathToHQFastest(t *map[string]*table.Territory, HQ string) {

	// var dist int64 = 1
	// var path []string
	var graph = dijkstra.NewGraph()
	var HQID int

	// find the id of hq
	for _, territory := range *t {
		if territory.Property.HQ {
			// fmt.Println(territory.ID)
			HQID = territory.ID
			break
		}
	}

	var vertexAdded = make(map[int]bool)

	for name := range *t {
		// Add logic to compute the shortest path to HQ using Dijkstra's algorithm
		// add current node
		if vertexAdded[(*t)[name].ID] {
			cl.Warn("Vertex ID :", (*t)[name].ID, "already added")
			continue
		} else {
			graph.AddVertex((*t)[name].ID)
			// log.Println("Added vertex ID:", territories[name].ID)
		}
	}

	// now add arc
	for _, territory := range *t {

		var currTerr = territory.ID
		// log.Println("Current territory ID:", currTerr, " ", territory.Name)
		for _, route := range territory.TradingRoutes {

			var currConn = (*t)[route].ID

			// log.Println("Connection ID", currConn, " ", route)

			// distance is always 1
			var err = graph.AddArc(currTerr, currConn, 1)
			if err != nil {
				cl.Error("Error adding arc :", err)
			}

		}
	}

	cl.Debug("HQID :", HQID)

	// get terr id
	for _, territory := range *t {

		if territory.Property.HQ {
			continue
		}

		var terrID = territory.ID
		var pathToHQRaw, err = graph.ShortestSafe(terrID, HQID)

		if err != nil {
			cl.Error("Error getting shortest path :", err)
		}

		// assign path to hq to the territory
		// convert terr id to terr name and store in pathToHQ
		var pathList = pathToHQRaw.Path
		var path = make([]string, len(pathList))
		var counter = 0
		for _, id := range pathList {
			for _, terr := range *t {
				if terr.ID == id {
					path[counter] = terr.Name
					counter += 1
				}
			}
		}
		counter = 0
		territory.RouteToHQ = path
		var reversePath = make([]string, len(path))
		copy(reversePath, path)
		arrutil.Reverse(reversePath)
		territory.RouteFromHQ = reversePath
	}
}

func CalculateRouteToHQTax(territories *map[string]*table.Territory, HQ string) {
	// Iterate through all territories and calculate the route tax to the HQ
	for _, territory := range *territories {
		// Skip the HQ territory
		if territory.Property.HQ {
			continue
		}

		// Calculate the route to the HQ
		routeToHQ := (*territory).RouteToHQ

		// Initialize the taxList
		taxList := []float64{}

		// Iterate through the route to HQ and get the tax of each territory
		for _, terr := range routeToHQ {
			if (*territories)[terr].Property.HQ {
				continue
			}

			if (*territories)[terr].Ally {

				// Use ally tax instead of others tax
				taxList = append(taxList, 1-float64((*territories)[terr].Property.Tax.Ally)/100)

			} else if (*territories)[terr].Claim {

				// if we claimed the territory, the tax should be 0
				taxList = append(taxList, 1)

			} else {

				// Use others tax
				taxList = append(taxList, 1-float64((*territories)[terr].Property.Tax.Others)/100)

			}

			// Calculate the route tax
			routeTax := 1.0
			for _, tax := range taxList {
				routeTax *= tax
			}

			routeTax *= 100

			// round it to 2 decimal places
			territory.RouteTax = math.Round((100-routeTax)*100) / 100
		}
	}
}

func CalculateTerritoryLevel(territories *map[string]*table.Territory) {
	for _, territory := range *territories {

		// add basic attack up
		var td = (*territory).Property.CurrentUpgrades
		var terrDef = td.Damage + td.Attack + td.Health + td.Defence

		// if aura is present
		if (*territory).Property.CurrentBonuses.TowerAura > 0 {
			terrDef += (*territory).Property.CurrentBonuses.TowerAura + 5
		}

		// if volley present
		if (*territory).Property.CurrentBonuses.TowerVolley > 0 {
			terrDef += (*territory).Property.CurrentBonuses.TowerVolley + 3
		}

		(*territory).RawLevel = terrDef

		// vlow = 0 - 5, low = 6 - 18, med = 19 - 30, high = 31 - 48, vhigh >= 49
		switch {
		case terrDef >= 0 && terrDef <= 5:
			(*territory).Level = "Very Low"
		case terrDef >= 6 && terrDef <= 18:
			(*territory).Level = "Low"
		case terrDef >= 19 && terrDef <= 30:
			(*territory).Level = "Medium"
		case terrDef >= 31 && terrDef <= 48:
			(*territory).Level = "High"
		case terrDef >= 49:
			(*territory).Level = "Very High"
		default:
			(*territory).Level = "An error has occured"
		}
	}
}

// for cshared

//export InitialiseData
func InitialiseData(id *C.char) int {

	var isDebug = os.Getenv("DEBUG")
	if isDebug == "true" {
		cl.SetDebug(true)
	}

	cl.Log("==== Eco Engine v0.0.1 (Debug build) ====")
	cl.Log("")
	// load all upgrades data
	var bytes, err = os.ReadFile("./upgrades.json")
	cl.Log("Loading upgrades data.")
	if err != nil {
		cl.Error("Failed to load upgrades data. :" + err.Error())
	}

	err = json.Unmarshal(bytes, &upgrades)
	if err != nil {
		cl.Error("Failed to deserialise upgrades data :" + err.Error())
	}

	var uninitTerritories map[string]table.RawTerritoryData
	bytes, err = os.ReadFile("./baseProperty.json")
	cl.Log("Loading base property data.")
	if err != nil {
		cl.Error("Failed to load base property data :" + err.Error())
	}

	err = json.Unmarshal(bytes, &uninitTerritories)
	if err != nil {
		cl.Error("Failed to deserialise base property data :" + err.Error())
	}

	// initialise territory
	cl.Log("Loading territories.")
	var territories = make(map[string]*table.Territory, len(uninitTerritories))
	var counter = 0
	for name, data := range uninitTerritories {
		territories[name] = &table.Territory{
			Name: name,
			BaseResourceProduction: table.TerritoryResource{
				Emerald: data.Resources.Emeralds,
				Ore:     data.Resources.Ore,
				Wood:    data.Resources.Wood,
				Fish:    data.Resources.Fish,
				Crop:    data.Resources.Crops,
			},
			Property: table.TerritoryProperty{
				TargetUpgrades: table.TerritoryPropertyUpgradeData{
					Damage:  11,
					Attack:  11,
					Health:  11,
					Defence: 11,
				},
				TargetBonuses: table.TerritoryPropertyBonusesData{
					StrongerMinions:       0,
					TowerMultiAttack:      0,
					TowerAura:             0,
					TowerVolley:           0,
					LargerResourceStorage: 0,
					LargerEmeraldStorage:  0,
					EfficientResource:     0,
					EfficientEmerald:      0,
					ResourceRate:          0,
					EmeraldRate:           0,
				},
				CurrentUpgrades: table.TerritoryPropertyUpgradeData{
					Damage:  0,
					Attack:  0,
					Health:  0,
					Defence: 0,
				},
				CurrentBonuses: table.TerritoryPropertyBonusesData{
					StrongerMinions:       0,
					TowerMultiAttack:      0,
					TowerAura:             0,
					TowerVolley:           0,
					LargerResourceStorage: 0,
					LargerEmeraldStorage:  0,
					EfficientResource:     0,
					EfficientEmerald:      0,
					ResourceRate:          0,
					EmeraldRate:           0,
				},
				Tax: table.Tax{
					Ally:   5,
					Others: 5,
				},
				Border:       "Open",
				TradingStyle: "Cheapest",
				HQ:           false,
			},
			Storage: table.TerritoryResourceStorage{
				Capacity: table.TerritoryResourceStorageValue{
					Emerald: 3000,
					Ore:     300,
					Wood:    300,
					Fish:    300,
					Crop:    300,
				},
				Current: table.TerritoryResourceStorageValue{
					Emerald: 0,
					Ore:     0,
					Wood:    0,
					Fish:    0,
					Crop:    0,
				},
			},
			ResourceProduction: table.TerritoryResource{
				Emerald: data.Resources.Emeralds,
				Ore:     data.Resources.Ore,
				Wood:    data.Resources.Wood,
				Fish:    data.Resources.Fish,
				Crop:    data.Resources.Crops,
			},
			TerritoryUsage: table.TerritoryResource{
				Emerald: 0,
				Ore:     0,
				Wood:    0,
				Fish:    0,
				Crop:    0,
			},
			TradingRoutes: data.TradingRoutes,
			ID:            counter,
		}
		counter++
		cl.Debug("Loaded territory", name, "with ID", territories[name].ID)
	}
	t = territories
	cl.Log("Loaded", len(t), "territories.")

	var tr struct {
		Territories []string `json:"territories"`
		HQ          string   `json:"hq"`
	}
	json.Unmarshal([]byte(C.GoString(id)), &tr)

	cl.Log("Initialising territories", tr.Territories, "with HQ", tr.HQ)

	hq = tr.HQ

	// if no hq provided
	if hq == "" {
		return 9
	}

	for _, name := range tr.Territories {

		// set all the territory properties to 0 or default
		t[name].Storage.Capacity = table.TerritoryResourceStorageValue{
			Emerald: 3000,
			Ore:     300,
			Wood:    300,
			Fish:    300,
			Crop:    300,
		}
		loadedTerritories[name] = t[name]
		if name == hq {
			cl.Debug("Setting HQ to", name)
			t[name].SetHQ()
		}
	}

	for _, terr := range tr.Territories {
		(t)[terr].Claim = true
	}

	var terrList = make(map[string]*table.Territory)
	for _, name := range tr.Territories {
		terrList[name] = t[name]
	}

	GetPathToHQCheapest(&t, hq)
	CalculateRouteToHQTax(&t, hq)

	cl.Log("Initialised", len(terrList), "territories with HQ at", hq)

	initialised = true
	return 0
}

//export Run
func Run() {
	GetPathToHQCheapest(&t, hq)
	CalculateRouteToHQTax(&t, hq)
	startTimer(&t, hq)
}

//export GetState
func GetState() *C.char {
	var bytes, _ = json.Marshal(t)
	var data = C.CString(string(bytes))

	return data
}

//export Ally
func Ally(a *C.char) int {
	var ad = C.GoString(a)
	var allyData struct {
		method    string
		territory string
	}

	var err = json.Unmarshal([]byte(ad), &allyData)
	if err != nil {
		return 1
	}

	var terr = allyData.territory

	if terr == "" {
		return 2
	}

	if allyData.method == "add" {
		t[terr].Ally = true
	} else {
		t[terr].Ally = false
	}

	return 0
}

//export UpdateTerritory
func UpdateTerritory(te *C.char, data *C.char) int {
	var tr string = C.GoString(te)
	var terrData *table.TerritoryUpdateData
	var err = json.Unmarshal([]byte(C.GoString(data)), &terrData)
	if err != nil {
		return 0
	}
	cl.Log(terrData)
	t[tr].Set(*terrData)

	return 1
}

//export UpdateTerritoryBulk
func UpdateTerritoryBulk(data string) int {

	var terrData struct {
		territories []string                    `json:"territories"`
		data        []table.TerritoryUpdateData `json:"data"`
	}

	for i, terr := range terrData.territories {
		t[terr].Set(terrData.data[i])
	}

	return 0
}

//export Undef
func Undef(te *C.char) int {
	tr := C.GoString(te)
	// check if tr is claimed by us
	if !t[tr].Claim {
		return 1
	}

	t[tr].Undefend()

	return 0
}