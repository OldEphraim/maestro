package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

type Scheduler struct {
	cron      *cron.Cron
	engine    *workflow.Engine
	agents    *agent.Store
	workflows *workflow.Store

	mu   sync.Mutex
	jobs map[string]cron.EntryID // schedule ID → cron entry ID
}

func New(engine *workflow.Engine, agents *agent.Store, workflows *workflow.Store) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		engine:    engine,
		agents:    agents,
		workflows: workflows,
		jobs:      make(map[string]cron.EntryID),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	if err := s.loadSchedules(ctx); err != nil {
		log.Printf("[scheduler] failed to load schedules: %v", err)
	}
	s.cron.Start()
	log.Println("[scheduler] started")

	go func() {
		<-ctx.Done()
		s.cron.Stop()
		log.Println("[scheduler] stopped")
	}()
}

func (s *Scheduler) Reload(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove all existing jobs
	for _, entryID := range s.jobs {
		s.cron.Remove(entryID)
	}
	s.jobs = make(map[string]cron.EntryID)

	if err := s.loadSchedules(ctx); err != nil {
		log.Printf("[scheduler] reload failed: %v", err)
	}
}

func (s *Scheduler) loadSchedules(ctx context.Context) error {
	agents, err := s.agents.List(ctx)
	if err != nil {
		return err
	}

	for _, ag := range agents {
		schedules, err := s.agents.GetSchedules(ctx, ag.ID)
		if err != nil {
			log.Printf("[scheduler] failed to get schedules for agent %s: %v", ag.Name, err)
			continue
		}
		for _, sch := range schedules {
			if !sch.Enabled {
				continue
			}
			s.registerJob(sch, ag)
		}
	}
	return nil
}

func (s *Scheduler) registerJob(sch agent.Schedule, ag agent.Agent) {
	scheduleID := sch.ID.String()
	agentID := ag.ID
	taskPrompt := sch.TaskPrompt

	entryID, err := s.cron.AddFunc(sch.CronExpr, func() {
		log.Printf("[scheduler] firing schedule %s for agent %s: %s", scheduleID, ag.Name, taskPrompt)
		ctx := context.Background()
		_, err := s.engine.ExecuteAdhoc(ctx, agentID, "schedule", taskPrompt)
		if err != nil {
			log.Printf("[scheduler] execute failed: %v", err)
			return
		}

		now := time.Now()
		// Compute next run from cron expression
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(sch.CronExpr)
		if err != nil {
			log.Printf("[scheduler] parse cron %q: %v", sch.CronExpr, err)
			return
		}
		nextRun := schedule.Next(now)
		s.agents.UpdateLastRun(ctx, sch.ID, now, nextRun)
	})

	if err != nil {
		log.Printf("[scheduler] failed to register cron %q for agent %s: %v", sch.CronExpr, ag.Name, err)
		return
	}

	s.mu.Lock()
	s.jobs[scheduleID] = entryID
	s.mu.Unlock()

	log.Printf("[scheduler] registered %q for agent %s (schedule %s)", sch.CronExpr, ag.Name, scheduleID)
}
