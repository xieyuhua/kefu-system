package service

import "time"

const TimeLayout = "2006-01-02 15:04:05"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(TimeLayout)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(TimeLayout)
}
