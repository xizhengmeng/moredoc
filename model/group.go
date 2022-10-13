package model

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Group struct {
	Id          int64     `form:"id" json:"id,omitempty" gorm:"primaryKey;autoIncrement;column:id;comment:用户组 id;"`
	Title       string    `form:"title" json:"title,omitempty" gorm:"column:title;type:varchar(64);size:64;index:title,unique;comment:用户组名称;"`
	Color       string    `form:"color" json:"color,omitempty" gorm:"column:color;type:varchar(20);size:20;comment:颜色;"`
	Icon        string    `form:"icon" json:"icon,omitempty" gorm:"column:icon;type:varchar(255);size:255;comment:icon;"`
	IsDefault   int8      `form:"is_default" json:"is_default,omitempty" gorm:"column:is_default;type:tinyint(3) unsigned;default:0;comment:是否默认;"`
	IsDisplay   int8      `form:"is_display" json:"is_display,omitempty" gorm:"column:is_display;type:tinyint(3) unsigned;default:0;comment:是否显示在用户名后;"`
	Description string    `form:"description" json:"description,omitempty" gorm:"column:description;type:varchar(255);size:255;comment:用户组描述;"`
	UserCount   int       `form:"user_count" json:"user_count,omitempty" gorm:"column:user_count;type:int(11);size:11;default:0;comment:用户数量;"`
	Sort        int       `form:"sort" json:"sort,omitempty" gorm:"column:sort;type:int(11);size:11;default:0;comment:排序，值越大越靠前;"`
	CreatedAt   time.Time `form:"created_at" json:"created_at,omitempty" gorm:"column:created_at;type:datetime;comment:创建时间;"`
	UpdatedAt   time.Time `form:"updated_at" json:"updated_at,omitempty" gorm:"column:updated_at;type:datetime;comment:更新时间;"`
}

// 这里是proto文件中的结构体，可以根据需要删除或者调整
//message Group {
// int64 id = 1;
// string title = 2;
// string color = 3;
// string icon = 4;
// int32 is_default = 5;
// int32 is_display = 6;
// string description = 7;
// int32 user_count = 8;
// int32 sort = 9;
//   = 0;
//   = 0;
//}

func (Group) TableName() string {
	return tablePrefix + "group"
}

// CreateGroup 创建Group
// TODO: 创建成功之后，注意相关表统计字段数值的增减
func (m *DBModel) CreateGroup(group *Group) (err error) {
	err = m.db.Create(group).Error
	if err != nil {
		m.logger.Error("CreateGroup", zap.Error(err))
		return
	}
	return
}

// UpdateGroup 更新Group，如果需要更新指定字段，则请指定updateFields参数
func (m *DBModel) UpdateGroup(group *Group, updateFields ...string) (err error) {
	db := m.db.Model(group)

	updateFields = m.FilterValidFields(Group{}.TableName(), updateFields...)
	if len(updateFields) > 0 { // 更新指定字段
		db = db.Select(updateFields)
	}

	err = db.Where("id = ?", group.Id).Updates(group).Error
	if err != nil {
		m.logger.Error("UpdateGroup", zap.Error(err))
	}
	return
}

// GetGroup 根据id获取Group
func (m *DBModel) GetGroup(id interface{}, fields ...string) (group Group, err error) {
	db := m.db

	fields = m.FilterValidFields(Group{}.TableName(), fields...)
	if len(fields) > 0 {
		db = db.Select(fields)
	}

	err = db.Where("id = ?", id).First(&group).Error
	return
}

func (m *DBModel) GetGroupByTitle(title string) (group Group, err error) {
	err = m.db.Where("title = ?", title).First(&group).Error
	return
}

type OptionGetGroupList struct {
	Page         int
	Size         int
	WithCount    bool                      // 是否返回总数
	Ids          []interface{}             // id列表
	SelectFields []string                  // 查询字段
	QueryRange   map[string][2]interface{} // map[field][]{min,max}
	QueryIn      map[string][]interface{}  // map[field][]{value1,value2,...}
	QueryLike    map[string][]interface{}  // map[field][]{value1,value2,...}
	Sort         []string
}

// GetGroupList 获取Group列表
func (m *DBModel) GetGroupList(opt OptionGetGroupList) (groupList []Group, total int64, err error) {
	db := m.db.Model(&Group{})

	for field, rangeValue := range opt.QueryRange {
		fields := m.FilterValidFields(Group{}.TableName(), field)
		if len(fields) == 0 {
			continue
		}
		if rangeValue[0] != nil {
			db = db.Where(fmt.Sprintf("%s >= ?", field), rangeValue[0])
		}
		if rangeValue[1] != nil {
			db = db.Where(fmt.Sprintf("%s <= ?", field), rangeValue[1])
		}
	}

	for field, values := range opt.QueryIn {
		fields := m.FilterValidFields(Group{}.TableName(), field)
		if len(fields) == 0 {
			continue
		}
		db = db.Where(fmt.Sprintf("%s in (?)", field), values)
	}

	for field, values := range opt.QueryLike {
		fields := m.FilterValidFields(Group{}.TableName(), field)
		if len(fields) == 0 {
			continue
		}
		db = db.Where(strings.TrimSuffix(fmt.Sprintf(strings.Join(make([]string, len(values)+1), "%s like ? or"), field), "or"), values...)
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id in (?)", opt.Ids)
	}

	if opt.WithCount {
		err = db.Count(&total).Error
		if err != nil {
			m.logger.Error("GetGroupList", zap.Error(err))
			return
		}
	}

	opt.SelectFields = m.FilterValidFields(Group{}.TableName(), opt.SelectFields...)
	if len(opt.SelectFields) > 0 {
		db = db.Select(opt.SelectFields)
	}

	if len(opt.Sort) > 0 {
		var sorts []string
		for _, sort := range opt.Sort {
			slice := strings.Split(sort, " ")
			if len(m.FilterValidFields(Group{}.TableName(), slice[0])) == 0 {
				continue
			}

			if len(slice) == 2 {
				sorts = append(sorts, fmt.Sprintf("%s %s", slice[0], slice[1]))
			} else {
				sorts = append(sorts, fmt.Sprintf("%s desc", slice[0]))
			}
		}
		if len(sorts) > 0 {
			db = db.Order(strings.Join(sorts, ","))
		}
	}

	db = db.Offset((opt.Page - 1) * opt.Size).Limit(opt.Size)

	err = db.Find(&groupList).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		m.logger.Error("GetGroupList", zap.Error(err))
	}
	return
}

// DeleteGroup 删除数据
// TODO: 删除数据之后，存在 group_id 的关联表，需要删除对应数据，同时相关表的统计数值，也要随着减少
func (m *DBModel) DeleteGroup(ids []interface{}) (err error) {
	err = m.db.Where("id in (?)", ids).Delete(&Group{}).Error
	if err != nil {
		m.logger.Error("DeleteGroup", zap.Error(err))
	}
	return
}

// GetDefaultUserGroup 获取默认的用户组
func (m *DBModel) GetDefaultUserGroup() (group Group, err error) {
	err = m.db.Where("is_default = ?", true).First(&group).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		m.logger.Error("GetDefaultUserGroup", zap.Error(err))
	}
	return
}