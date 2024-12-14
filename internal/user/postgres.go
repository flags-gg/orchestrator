package user

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Group struct {
	Id   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

type User struct {
	Id        *string `json:"id,omitempty"`
	KnownAs   *string `json:"known_as,omitempty"`
	Email     *string `json:"email_address,omitempty"`
	Subject   *string `json:"subject,omitempty"`
	Timezone  *string `json:"timezone,omitempty"`
	JobTitle  *string `json:"job_title,omitempty"`
	Location  *string `json:"location,omitempty"`
	Avatar    *string `json:"avatar,omitempty"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	UserGroup *Group  `json:"user_group,omitempty"`
	Onboarded *bool   `json:"onboarded,omitempty"`
	Created   bool    `json:"created,omitempty"`
}

type Notification struct {
	Id        *string    `json:"id,omitempty"`
	Subject   *string    `json:"subject,omitempty"`
	Content   *string    `json:"content,omitempty"`
	Read      *bool      `json:"read,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	Action    *string    `json:"action,omitempty"`
}

type Notifications struct {
	Notifications []Notification `json:"notifications,omitempty"`
}

func (s *System) CreateUserDetails(subject, knownAs, email, firstname, lastname, location string, userGroup int) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
    INSERT INTO public.user (
        subject,
        email_address,
        first_name,
        last_name,
        known_as,
        user_group_id,
        location
    ) VALUES ($1, $2, $3, $4, $5, $6, $7)
    ON CONFLICT (subject) DO UPDATE SET
        email_address = $2,
        first_name = $3,
        last_name = $4,
        known_as = $5,
        location = $7`, subject, email, firstname, lastname, knownAs, userGroup, location); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert user into database: %v", err)
	}

	return nil
}

func (s *System) RetrieveUserDetailsDB(subject string) (*User, error) {
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
	ug := &Group{}
	if err := client.QueryRow(s.Context, `
    SELECT
		u.id,
        u.known_as,
        u.email_address,
        u.subject,
        u.timezone,
        u.job_title,
        u.location,
        u.avatar,
        u.first_name,
        u.last_name,
        u.user_group_id,
        u.onboarded,
    	ug.name AS user_group_name
    FROM public.user AS u
    	LEFT JOIN public.user_groups AS ug ON ug.id = u.user_group_id
    	LEFT JOIN public.company_user AS cu ON cu.user_id = u.id
    WHERE subject = $1`, subject).Scan(&user.Id, &user.KnownAs, &user.Email, &user.Subject, &user.Timezone, &user.JobTitle, &user.Location, &user.Avatar, &user.FirstName, &user.LastName, &ug.Id, &user.Onboarded, &ug.Name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	user.UserGroup = ug

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

	rows, err := client.Query(s.Context, `
    SELECT
      un.id,
      un.subject,
      "content",
      "action",
      "read",
      un.created_at
    FROM public.user_notifications AS un
      JOIN public.user AS u ON u.id = un.user_id
    WHERE u.subject = $1
      AND deleted = false`, subject)
	if err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
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

	if _, err := client.Exec(s.Context, `
    UPDATE public.user_notifications AS un
    SET "read" = true
    FROM public.user AS u
    WHERE un.user_id = u.id
        AND u.subject = $1
        AND un.id = $2`, subject, notificationId); err != nil {
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

	if _, err := client.Exec(s.Context, `
    UPDATE public.user_notifications AS un
    SET
      deleted = true,
      "read" = true
    FROM public.user AS u
    WHERE un.user_id = u.id
        AND u.subject = $1
        AND un.id = $2`, subject, notificationId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
	}

	return nil
}

func (s *System) UpdateUserImageInDB(subject string, image string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `UPDATE public.user
		SET avatar = $1
		WHERE subject = $2`, image, subject); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user image: %v", err)
	}

	return nil
}

func (s *System) UpdateUserDetailsDB(subject, knownAs, email, firstname, lastname, location string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `
    UPDATE public.user
    SET
      known_as = $1,
      email_address = $2,
      first_name = $3,
      last_name = $4,
      location = $6
    WHERE subject = $5`, knownAs, email, firstname, lastname, subject, location)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user details: %v", err)
	}

	return nil
}

func (s *System) DeleteUserInDB(subject string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
	WITH deleted_user AS (
        DELETE FROM public.user 
        WHERE subject = $1
        RETURNING id
    )
    DELETE FROM public.company_user 
    WHERE user_id IN (SELECT id FROM deleted_user)`, subject); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete user from company_user: %v", err)
	}

	return nil
}
