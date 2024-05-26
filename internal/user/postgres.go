package user

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type User struct {
	Id      *string `json:"id"`
	KnownAs *string `json:"known_as"`
	Email   *string `json:"email_address"`
	Subject *string `json:"subject"`
}

type Notification struct {
	Id        *string    `json:"id"`
	Subject   *string    `json:"subject"`
	Content   *string    `json:"content"`
	Read      *bool      `json:"read"`
	CreatedAt *time.Time `json:"created_at"`
	Action    *string    `json:"action"`
}

type Notifications struct {
	Notifications []Notification `json:"notifications,omitempty"`
}

func (s *System) CreateUserDetails(subject, email string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, "INSERT INTO public.user (subject, email_address) VALUES ($1, $2)", subject, email); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert user into database: %v", err)
	}

	return nil
}

func (s *System) RetrieveUserDetails(subject string) (*User, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	user := &User{}
	if err := client.QueryRow(s.Context, "SELECT id, known_as, email_address, subject FROM public.user WHERE subject = $1", subject).Scan(&user.Id, &user.KnownAs, &user.Email, &user.Subject); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return user, nil
}

func (s *System) RetrieveUserNotifications(subject string) ([]Notification, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, `SELECT un.id, un.subject, "content", "action", "read", un.created_at FROM public.user_notifications AS un JOIN public.user AS u ON u.id = un.user_id WHERE u.subject = $1 AND deleted = false`, subject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var notification Notification
		if err := rows.Scan(&notification.Id, &notification.Subject, &notification.Content, &notification.Action, &notification.Read, &notification.CreatedAt); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
		}
		notifications = append(notifications, notification)
	}

	return notifications, nil
}

func (s *System) MarkNotificationAsRead(subject, notificationId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `UPDATE public.user_notifications AS un SET "read" = true FROM public.user AS u WHERE un.user_id = u.id AND u.subject = $1 AND un.id = $2`, subject, notificationId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
	}

	return nil
}

func (s *System) DeleteUserNotificationInDB(subject, notificationId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `UPDATE public.user_notifications AS un SET deleted = true, "read" = true FROM public.user AS u WHERE un.user_id = u.id AND u.subject = $1 AND un.id = $2`, subject, notificationId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
	}

	return nil
}
