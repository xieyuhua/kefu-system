package util

import "time"

// TimeLayout 响应体里所有时间字段的统一格式
const TimeLayout = "2006-01-02 15:04:05"

// FmtTime 格式化时间；零值返回空串，避免前端显示 0001-01-01
func (u *Util) FmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(TimeLayout)
}

// FmtTimePtr 格式化时间指针；nil 返回空串
func (u *Util) FmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(TimeLayout)
}
