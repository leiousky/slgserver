package logic

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"math/rand"
	"slgserver/log"
	"slgserver/server/logic/mgr"
	"slgserver/server/model"
	"slgserver/server/proto"
	"slgserver/server/static_conf"
	"slgserver/server/static_conf/facility"
	"slgserver/server/static_conf/general"
	"slgserver/util"
	"time"
)

//战斗位置的属性
type armyPosition struct {
	general  *model.General
	soldiers int //兵力
	force    int //武力
	strategy int //策略
	defense  int //防御
	speed    int //速度
	destroy  int //破坏
	arms     int //兵种
	position int //位置
}

const maxRound = 10

type armyWar struct {
	attack *model.Army
	defense *model.Army
	attackPos []*armyPosition
	defensePos []*armyPosition
}

type battle struct {
	AId   int `json:"a_id"`   //本回合发起攻击的武将id
	DId   int `json:"d_id"`   //本回合防御方的武将id
	ALoss int `json:"a_loss"` //本回合攻击方损失的兵力
	DLoss int `json:"d_loss"` //本回合防守方损失的兵力
}

func (this* battle) to() []int{
	r := make([]int, 0)
	r = append(r, this.AId)
	r = append(r, this.DId)
	r = append(r, this.ALoss)
	r = append(r, this.DLoss)
	return r
}

type warRound struct {
	Battle	[][]int	`json:"b"`
}

type WarResult struct {
	round 	[]*warRound
	result	int			//0失败，1平，2胜利
}

func NewWar(attack *model.Army, defense *model.Army) *WarResult {

	w := armyWar{attack: attack, defense: defense}
	w.init()
	wars := w.battle()

	result := &WarResult{round: wars}
	if w.attackPos[0].soldiers == 0{
		result.result = 0
	}else if w.defensePos[0].soldiers != 0{
		result.result = 1
	}else{
		result.result = 2
	}

	return result
}

//初始化军队和武将属性、兵种、加成等
func (this* armyWar) init() {

	//城内设施加成
	attackAdds := []int{0,0,0,0}
	if this.attack.CityId > 0{
		attackAdds = mgr.RFMgr.GetAdditions(this.attack.CityId,
			facility.TypeForce,
			facility.TypeDefense,
			facility.TypeSpeed,
			facility.TypeStrategy)
	}

	defenseAdds := []int{0,0,0,0}
	if this.defense.CityId > 0{
		defenseAdds = mgr.RFMgr.GetAdditions(this.defense.CityId,
			facility.TypeForce,
			facility.TypeDefense,
			facility.TypeSpeed,
			facility.TypeStrategy)
	}

	//阵营加成
	aCampAdds := []int{0}
	aCamp := this.attack.GetCamp()
	if aCamp > 0{
		aCampAdds = mgr.RFMgr.GetAdditions(this.attack.CityId, facility.TypeHanAddition-1+aCamp)
	}

	dCampAdds := []int{0}
	dCamp := this.attack.GetCamp()
	if dCamp > 0 {
		dCampAdds = mgr.RFMgr.GetAdditions(this.defense.CityId, facility.TypeHanAddition-1+aCamp)
	}

	this.attackPos = make([]*armyPosition, 0)
	this.defensePos = make([]*armyPosition, 0)

	for i, g := range this.attack.Gens {
		if g == nil {
			this.attackPos = append(this.attackPos, nil)
		}else{
			pos := &armyPosition{
				general:  g,
				soldiers: this.attack.SoldierArray[i],
				force:    g.GetForce()  + attackAdds[0] + aCampAdds[0],
				defense:  g.GetDefense() + attackAdds[1] + aCampAdds[0],
				speed:    g.GetSpeed() + attackAdds[2] + aCampAdds[0],
				strategy: g.GetStrategy() + attackAdds[3] + aCampAdds[0],
				destroy:  g.GetDestroy() + aCampAdds[0],
				arms:     g.CurArms,
				position: i,
			}
			this.attackPos = append(this.attackPos, pos)
		}
	}

	for i, g := range this.defense.Gens {
		if g == nil {
			this.defensePos = append(this.defensePos, nil)
		}else{
			pos := &armyPosition{
				general:  g,
				soldiers: this.defense.SoldierArray[i],
				force:    g.GetForce() + defenseAdds[0] + dCampAdds[0],
				defense:  g.GetDefense() + defenseAdds[1] + dCampAdds[0],
				speed:    g.GetSpeed() + defenseAdds[2] + dCampAdds[0],
				strategy: g.GetStrategy() + defenseAdds[3] + dCampAdds[0],
				destroy:  g.GetDestroy() + dCampAdds[0],
				arms:     g.CurArms,
				position: i,
			}
			this.defensePos = append(this.defensePos, pos)
		}
	}



	fmt.Println(this.defensePos)
}

func (this* armyWar) battle() []*warRound{
	rounds := make([]*warRound, 0)
	cur := 0
	for true{
		r, isEnd := this.round()
		rounds = append(rounds, r)
		cur += 1
		if cur >= maxRound || isEnd{
			break
		}
	}
	return rounds
}

//回合
func (this* armyWar) round() (*warRound, bool) {

	war := &warRound{}
	n := rand.Intn(10)
	attack := this.attackPos
	defense := this.defensePos
	attackArmy := this.attack
	defenseArmy := this.defense

	isEnd := false
	//随机先手
	if n % 2 == 0{
		attack = this.defensePos
		defense = this.attackPos

		attackArmy = this.defense
		defenseArmy = this.attack
	}

	//攻击方回合
	for _, posAttack := range attack {
		if posAttack == nil || posAttack.soldiers == 0{
			continue
		}

		//计算
		posDefense, index := this.randArmyPosition(defense)
		if posDefense == nil{
			isEnd = true
			goto end
		}

		hurm := posAttack.soldiers *posAttack.force /1000
		def := posDefense.soldiers *posDefense.defense /1000

		kill := hurm-def
		if kill > 0{
			kill = util.MinInt(kill, posDefense.soldiers)
			posDefense.soldiers -= kill
			defenseArmy.SoldierArray[index] -= kill
			posAttack.general.Exp += kill*5
		}else{
			kill = 0
		}

		b := battle{AId: posAttack.general.Id, ALoss: 0, DId: posDefense.general.Id, DLoss: kill}
		war.Battle = append(war.Battle, b.to())

		//大营干死了，直接结束
		if posDefense.position == 0 && posDefense.soldiers == 0 {
			isEnd = true
			goto end
		}
	}

	//防守方回合
	for _, posAttack := range defense {
		if posAttack == nil || posAttack.soldiers == 0{
			continue
		}

		//计算
		posDefense, index := this.randArmyPosition(attack)
		if posDefense == nil{
			isEnd = true
			goto end
		}

		hurm := posAttack.soldiers *posAttack.force /10000
		def := posDefense.soldiers *posDefense.defense /10000

		kill := hurm-def
		if kill > 0{
			kill = util.MinInt(kill, posDefense.soldiers)
			posDefense.soldiers -= kill
			attackArmy.SoldierArray[index] -= kill
			posAttack.general.Exp += kill*10
		}else{
			kill = 0
		}

		b := battle{AId: posAttack.general.Id, ALoss: 0, DId: posDefense.general.Id, DLoss: kill}
		war.Battle = append(war.Battle, b.to())

		//大营干死了，直接结束
		if posDefense.position == 0 && posDefense.soldiers == 0 {
			isEnd = true
			goto end
		}
	}

	end:
	return war, isEnd
}

//随机一个目标队伍
func (this* armyWar) randArmyPosition(pos []*armyPosition) (*armyPosition, int){
	isEmpty := true
	for _, v := range pos {
		if v != nil && v.soldiers != 0 {
			isEmpty = false
			break
		}
	}

	if isEmpty {
		return nil, -1
	}


	for true {
		r := rand.Intn(100)
		index := r % len(pos)
		if pos[index] != nil && pos[index].soldiers != 0{
			return pos[index], index
		}
	}

	return nil, -1
}

func NewEmptyWar(attack *model.Army) *model.WarReport {
	//战报处理
	pArmy := attack.ToProto().(proto.Army)
	begArmy, _ := json.Marshal(pArmy)

	//武将战斗前
	begGeneral := make([][]int, 0)
	for _, g := range attack.Gens {
		if g != nil {
			pg := g.ToProto().(proto.General)
			begGeneral = append(begGeneral, pg.ToArray())
		}
	}
	begGeneralData, _ := json.Marshal(begGeneral)

	wr := &model.WarReport{X: attack.ToX, Y: attack.ToY, AttackRid: attack.RId,
		AttackIsRead: false, DefenseIsRead: true, DefenseRid: 0,
		BegAttackArmy: string(begArmy), BegDefenseArmy: "",
		EndAttackArmy: string(begArmy), EndDefenseArmy: "",
		BegAttackGeneral: string(begGeneralData),
		EndAttackGeneral: string(begGeneralData),
		BegDefenseGeneral: "",
		EndDefenseGeneral: "",
		Rounds: "",
		Result: 0,
		CTime: time.Now(),
	}
	return wr
}

//简单战斗
func newBattle(attackArmy *model.Army) {
	city, ok := mgr.RCMgr.PositionCity(attackArmy.ToX, attackArmy.ToY)
	if ok {
		//打玩家城池
		var enemys []*model.Army
		//驻守队伍被打
		posId := mgr.ToPosition(attackArmy.ToX, attackArmy.ToY)
		posArmys, ok := ArmyLogic.stopInPosArmys[posId]
		if ok {
			for _, army := range posArmys {
				enemys = append(enemys, army)
			}
		}

		//城内空闲的队伍被打
		if armys, ok := mgr.AMgr.GetByCity(city.CityId); ok {
			for _, enemy := range armys {
				if enemy.Cmd == model.ArmyCmdIdle{
					enemys = append(enemys, enemy)
				}
			}
		}

		if len(enemys) == 0 {
			//没有队伍
			destory := mgr.GMgr.GetDestroy(attackArmy)
			city.CurDurable = util.MaxInt(0, city.CurDurable - destory)
			city.SyncExecute()
		}else{
			lastWar, warReports := trigger(attackArmy, enemys, true)
			if lastWar.result > 1 {
				destory := mgr.GMgr.GetDestroy(attackArmy)
				wr := warReports[len(warReports)-1]
				wr.DestroyDurable = util.MinInt(destory, city.CurDurable)
				city.CurDurable = util.MaxInt(0, city.CurDurable - destory)
				if city.CurDurable == 0{
					aAttr, _ := mgr.RAttributeMgr.Get(attackArmy.RId)
					if aAttr.UnionId != 0{
						//有联盟才能俘虏玩家
						wr.Occupy = 1
						dAttr, _ := mgr.RAttributeMgr.Get(city.RId)
						dAttr.ParentId = aAttr.UnionId
						dAttr.SyncExecute()

					}else {
						wr.Occupy = 0
					}
				}else{
					wr.Occupy = 0
				}
				city.SyncExecute()
			}
			for _, wr := range warReports {
				wr.SyncExecute()
			}
		}
	}else{
		//打建筑
		executeBuild(attackArmy)
	}

}

func trigger(army *model.Army, enemys []*model.Army, isRoleEnemy bool) (*WarResult, []*model.WarReport) {

	posId := mgr.ToPosition(army.ToX, army.ToY)
	warReports := make([]*model.WarReport, 0)
	var lastWar *WarResult = nil

	for _, enemy := range enemys {
		//战报处理
		pArmy := army.ToProto().(proto.Army)
		pEnemy := enemy.ToProto().(proto.Army)

		begArmy1, _ := json.Marshal(pArmy)
		begArmy2, _ := json.Marshal(pEnemy)

		//武将战斗前
		begGeneral1 := make([][]int, 0)
		for _, g := range army.Gens {
			if g != nil {
				pg := g.ToProto().(proto.General)
				begGeneral1 = append(begGeneral1, pg.ToArray())
			}
		}
		begGeneralData1, _ := json.Marshal(begGeneral1)

		begGeneral2 := make([][]int, 0)
		for _, g := range enemy.Gens {
			if g != nil {
				pg := g.ToProto().(proto.General)
				begGeneral2 = append(begGeneral2, pg.ToArray())
			}
		}
		begGeneralData2, _ := json.Marshal(begGeneral2)

		lastWar = NewWar(army, enemy)

		//武将战斗后
		endGeneral1 := make([][]int, 0)
		for _, g := range army.Gens {
			if g != nil {
				pg := g.ToProto().(proto.General)
				endGeneral1 = append(endGeneral1, pg.ToArray())
				level, exp := general.GenBasic.ExpToLevel(g.Exp)
				g.Level = level
				g.Exp = exp
				g.SyncExecute()
			}
		}
		endGeneralData1, _ := json.Marshal(endGeneral1)

		endGeneral2 := make([][]int, 0)
		for _, g := range enemy.Gens {
			if g != nil {
				pg := g.ToProto().(proto.General)
				endGeneral2 = append(endGeneral2, pg.ToArray())
				level, exp := general.GenBasic.ExpToLevel(g.Exp)
				g.Level = level
				g.Exp = exp
				g.SyncExecute()
			}
		}
		endGeneralData2, _ := json.Marshal(endGeneral2)

		pArmy = army.ToProto().(proto.Army)
		pEnemy = enemy.ToProto().(proto.Army)
		endArmy1, _ := json.Marshal(pArmy)
		endArmy2, _ := json.Marshal(pEnemy)

		rounds, _ := json.Marshal(lastWar.round)
		wr := &model.WarReport{X: army.ToX, Y: army.ToY, AttackRid: army.RId,
			AttackIsRead: false, DefenseIsRead: false, DefenseRid: enemy.RId,
			BegAttackArmy: string(begArmy1), BegDefenseArmy: string(begArmy2),
			EndAttackArmy: string(endArmy1), EndDefenseArmy: string(endArmy2),
			BegAttackGeneral: string(begGeneralData1),
			BegDefenseGeneral: string(begGeneralData2),
			EndAttackGeneral: string(endGeneralData1),
			EndDefenseGeneral: string(endGeneralData2),
			Rounds: string(rounds),
			Result: lastWar.result,
			CTime: time.Now(),
		}

		warReports = append(warReports, wr)
		enemy.ToSoldier()
		enemy.ToGeneral()

		if isRoleEnemy {
			if lastWar.result > 1 {
				if isRoleEnemy {
					delete(ArmyLogic.stopInPosArmys, posId)
				}
				ArmyLogic.ArmyBack(enemy)
			}
			enemy.SyncExecute()
		}else{
			wr.DefenseIsRead = true
		}
	}
	army.SyncExecute()
	return lastWar, warReports
}

func executeBuild(army *model.Army)  {
	roleBuid, _ := mgr.RBMgr.PositionBuild(army.ToX, army.ToY)

	posId := mgr.ToPosition(army.ToX, army.ToY)
	posArmys, isRoleEnemy := ArmyLogic.stopInPosArmys[posId]

	var enemys []*model.Army
	if isRoleEnemy == false {
		enemys = ArmyLogic.sys.GetArmy(army.ToX, army.ToY)
	}else{
		for _, v := range posArmys {
			enemys = append(enemys, v)
		}
	}

	lastWar, warReports := trigger(army, enemys, isRoleEnemy)

	if lastWar.result > 1 {
		if roleBuid != nil {
			destory := mgr.GMgr.GetDestroy(army)
			wr := warReports[len(warReports)-1]
			wr.DestroyDurable = util.MinInt(destory, roleBuid.CurDurable)
			roleBuid.CurDurable = util.MaxInt(0, roleBuid.CurDurable - destory)
			if roleBuid.CurDurable == 0{
				//攻占了玩家的领地
				blimit := static_conf.Basic.Role.BuildLimit
				if blimit > mgr.RBMgr.BuildCnt(army.RId){
					wr.Occupy = 1
				}else{
					wr.Occupy = 0
				}
				mgr.RBMgr.RemoveFromRole(roleBuid)
				mgr.RBMgr.AddBuild(army.RId, army.ToX, army.ToY)
				roleBuid.CurDurable = roleBuid.MaxDurable
				OccupyRoleBuild(army.RId, army.ToX, army.ToY)
			}else{
				wr.Occupy = 0
			}

		}else{
			//占领系统领地
			wr := warReports[len(warReports)-1]
			blimit := static_conf.Basic.Role.BuildLimit
			if blimit > mgr.RBMgr.BuildCnt(army.RId){
				OccupySystemBuild(army.RId, army.ToX, army.ToY)
				wr.DestroyDurable = 100
				wr.Occupy = 1
			}else{
				wr.Occupy = 0
			}
			ArmyLogic.sys.DelArmy(army.ToX, army.ToY)
		}
	}

	//领地发生变化
	if newRoleBuild, ok := mgr.RBMgr.PositionBuild(army.ToX, army.ToY); ok {
		mgr.RoleBuildExtra(newRoleBuild)
		newRoleBuild.SyncExecute()
	}

	for _, wr := range warReports {
		wr.SyncExecute()
	}
}

func OccupyRoleBuild(rid, x, y int)  {
	newId := rid

	if b, ok := mgr.RBMgr.PositionBuild(x, y); ok {

		oldId := b.RId
		log.DefaultLog.Info("battle in role build",
			zap.Int("oldRId", oldId),
			zap.Int("newRId", newId))

		//被占领的减产
		if oldRole, ok := mgr.RResMgr.Get(oldId); ok{
			oldRole.WoodYield -= b.Wood
			oldRole.GrainYield -= b.Grain
			oldRole.StoneYield -= b.Stone
			oldRole.IronYield -= b.Iron
			oldRole.SyncExecute()
		}
		//占领的增加产量
		if newRole, ok := mgr.RResMgr.Get(newId); ok{
			newRole.WoodYield += b.Wood
			newRole.GrainYield += b.Grain
			newRole.StoneYield += b.Stone
			newRole.IronYield += b.Iron
			newRole.SyncExecute()
		}
		b.RId = rid
	}
}

func OccupySystemBuild(rid, x, y int)  {
	newId := rid

	if _, ok := mgr.RBMgr.PositionBuild(x, y); ok {
		return
	}

	if mgr.NMMgr.IsCanBuild(x, y){
		rb, ok := mgr.RBMgr.AddBuild(rid, x, y)
		if ok {
			//占领的增加产量
			if newRole, ok := mgr.RResMgr.Get(newId); ok{
				newRole.WoodYield += rb.Wood
				newRole.GrainYield += rb.Grain
				newRole.StoneYield += rb.Stone
				newRole.IronYield += rb.Iron
				newRole.SyncExecute()
			}
		}
	}
}