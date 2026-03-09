package agents

import (
	"log/slog"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/robfig/cron/v3"
)

// ScheduledTask represents a cron-based agent task.
type ScheduledTask struct {
	Config    config.ScheduledAgentConfig
	Schedule  cron.Schedule
	LastFired time.Time
}

// Scheduler manages cron-based agent task scheduling.
type Scheduler struct {
	tasks  []ScheduledTask
	parser cron.Parser
}

func NewScheduler(configs []config.ScheduledAgentConfig) *Scheduler {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	s := &Scheduler{parser: parser}

	for _, cfg := range configs {
		sched, err := parser.Parse(cfg.Cron)
		if err != nil {
			slog.Error("invalid cron expression", "name", cfg.Name, "cron", cfg.Cron, "err", err)
			continue
		}
		s.tasks = append(s.tasks, ScheduledTask{
			Config:   cfg,
			Schedule: sched,
		})
	}
	return s
}

// DueTasks returns tasks that are due to fire now.
// Implements double-fire prevention: won't fire again within the same minute.
func (s *Scheduler) DueTasks(now time.Time) []ScheduledTask {
	var due []ScheduledTask
	for i := range s.tasks {
		task := &s.tasks[i]
		next := task.Schedule.Next(task.LastFired)
		if next.Before(now) || next.Equal(now) {
			// Double-fire prevention: check if we already fired this minute
			if task.LastFired.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
				continue
			}
			task.LastFired = now
			due = append(due, *task)
		}
	}
	return due
}

// NextFireTime returns the next scheduled fire time for a task.
func (s *Scheduler) NextFireTime(name string) *time.Time {
	for _, task := range s.tasks {
		if task.Config.Name == name {
			t := task.Schedule.Next(time.Now())
			return &t
		}
	}
	return nil
}
