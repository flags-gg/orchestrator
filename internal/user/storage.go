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

func (u *System) CreateUserDetails(subject, email string) error {
	client, err := pgx.Connect(u.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", u.Config.Database.User, u.Config.Database.Password, u.Config.Database.Host, u.Config.Database.Port, u.Config.Database.DBName))
	if err != nil {
		return logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(u.Context); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(u.Context, "INSERT INTO public.user (subject, email_address) VALUES ($1, $2)", subject, email); err != nil {
		return logs.Errorf("Failed to insert user into database: %v", err)
	}

	return nil
}

func (u *System) RetrieveUserDetails(subject string) (*User, error) {
	client, err := pgx.Connect(u.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", u.Config.Database.User, u.Config.Database.Password, u.Config.Database.Host, u.Config.Database.Port, u.Config.Database.DBName))
	if err != nil {
		return nil, logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(u.Context); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	user := &User{}
	if err := client.QueryRow(u.Context, "SELECT id, known_as, email_address, subject FROM public.user WHERE subject = $1", subject).Scan(&user.Id, &user.KnownAs, &user.Email, &user.Subject); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, logs.Errorf("Failed to query database: %v", err)
	}

	return user, nil
}
