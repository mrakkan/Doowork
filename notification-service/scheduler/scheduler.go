package scheduler

import (
	"log"
	"time"

	"notification-service/models"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Scheduler struct {
	db   *gorm.DB
	cron *cron.Cron
}

func NewScheduler(db *gorm.DB) *Scheduler {
	return &Scheduler{
		db:   db,
		cron: cron.New(),
	}
}

func (s *Scheduler) Start() {
	// Check for pending notifications every minute
	s.cron.AddFunc("* * * * *", func() {
		s.processScheduledNotifications()
	})

	s.cron.Start()
	log.Println("Notification scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Notification scheduler stopped")
}

func (s *Scheduler) processScheduledNotifications() {
	now := time.Now()

	var scheduled []models.ScheduledNotification
	if err := s.db.Where("status = 'pending' AND scheduled_at <= ?", now).Find(&scheduled).Error; err != nil {
		log.Printf("Error fetching scheduled notifications: %v", err)
		return
	}

	for _, sn := range scheduled {
		// Create the actual notification
		notification := models.Notification{
			UserID:  sn.UserID,
			Title:   sn.Title,
			Message: sn.Message,
			Type:    sn.Type,
			Read:    false,
			Data:    sn.Data,
		}

		if err := s.db.Create(&notification).Error; err != nil {
			log.Printf("Error creating notification from scheduled: %v", err)
			continue
		}

		// Update scheduled notification status
		sentAt := time.Now()
		sn.Status = "sent"
		sn.SentAt = &sentAt
		s.db.Save(&sn)

		// Handle recurring notifications
		if sn.Recurring && sn.CronSchedule != "" {
			// Create next scheduled notification
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			schedule, err := parser.Parse(sn.CronSchedule)
			if err != nil {
				log.Printf("Error parsing cron schedule: %v", err)
				continue
			}

			nextTime := schedule.Next(now)
			newScheduled := models.ScheduledNotification{
				UserID:       sn.UserID,
				Title:        sn.Title,
				Message:      sn.Message,
				Type:         sn.Type,
				ScheduledAt:  nextTime,
				Recurring:    true,
				CronSchedule: sn.CronSchedule,
				Status:       "pending",
				Data:         sn.Data,
			}
			s.db.Create(&newScheduled)
		}

		log.Printf("Sent scheduled notification %d to user %d", sn.ID, sn.UserID)
	}
}
