package user

import (
	"errors"
	"fmt"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

type User struct {
	Id      *string `json:"id"`
	KnownAs *string `json:"known_as"`
	Email   *string `json:"email_address"`
	Subject *string `json:"subject"`
}

func (s *System) CreateUserDetails(subject, email string) error {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, "INSERT INTO public.user (subject, email_address) VALUES ($1, $2)", subject, email); err != nil {
		return logs.Errorf("Failed to insert user into database: %v", err)
	}

	return nil
}

func (s *System) RetrieveUserDetails(subject string) (*User, error) {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return nil, logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	user := &User{}
	if err := client.QueryRow(s.Context, "SELECT id, known_as, email_address, subject FROM public.user WHERE subject = $1", subject).Scan(&user.Id, &user.KnownAs, &user.Email, &user.Subject); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, logs.Errorf("Failed to query database: %v", err)
	}

	return user, nil
}
