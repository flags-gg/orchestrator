package pricing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/docker/go-connections/nat"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	ConfigBuilder "github.com/keloran/go-config"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type testContainer struct {
	container testcontainers.Container
	uri       string
}

func setupTestDatabase(c context.Context) (*testContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:14-alpine",
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections"),
			wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
				return fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
			}),
		).WithDeadline(time.Minute * 2),
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
		},
	}

	container, err := testcontainers.GenericContainer(c, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(c, "5432")
	if err != nil {
		return nil, err
	}

	hostIP, err := container.Host(c)
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", hostIP, mappedPort.Port())
	_ = os.Setenv("RDS_HOSTNAME", hostIP)
	_ = os.Setenv("RDS_PORT", mappedPort.Port())
	_ = os.Setenv("RDS_USERNAME", "test")
	_ = os.Setenv("RDS_PASSWORD", "test")
	_ = os.Setenv("RDS_DB", "testdb")

	// Initialize the database schema
	db, err := sql.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	// Create the payment_plans table
	_, err = db.Exec(`
		CREATE TABLE public.payment_plans (
			id serial NOT NULL,
			created_at timestamp without time zone NOT NULL DEFAULT now(),
			price integer NOT NULL,
			team_members integer NOT NULL,
			projects integer NOT NULL,
			agents integer NOT NULL,
			environments integer NOT NULL,
			requests integer NOT NULL,
			support_category character varying(255) NOT NULL,
			name character varying(255) NOT NULL,
			custom boolean NOT NULL DEFAULT false,
			popular boolean NOT NULL DEFAULT false,
			stripe_id character varying(255) NULL,
			stripe_id_dev character varying(255) NULL
		)`)
	if err != nil {
		return nil, err
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO public.payment_plans 
		(name, price, team_members, projects, agents, environments, requests, support_category, stripe_id, stripe_id_dev, custom, popular)
		VALUES
		('free', 0, 1, 1, 1, 1, 1000, 'community', 'free_stripe_id', 'free_dev_stripe_id', false, false),
		('startup', 29, 5, 3, 3, 3, 10000, 'email', 'startup_stripe_id', 'startup_dev_stripe_id', false, true),
		('pro', 99, 10, 10, 10, 5, 100000, 'priority', 'pro_stripe_id', 'pro_dev_stripe_id', false, false),
		('enterprise', 299, 50, 50, 50, 10, 1000000, 'dedicated', 'enterprise_stripe_id', 'enterprise_dev_stripe_id', false, false)
	`)
	if err != nil {
		return nil, err
	}

	return &testContainer{
		container: container,
		uri:       uri,
	}, nil
}

func setupTestSystem(t *testing.T) *System {
	c := ConfigBuilder.NewConfigNoVault()
	if err := c.Build(ConfigBuilder.Database); err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	return NewSystem(c)
}

func TestGetCompanyPricing(t *testing.T) {
	c := context.Background()

	testDB, err := setupTestDatabase(c)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() {
		if err := testDB.container.Terminate(c); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	system := setupTestSystem(t)
	system.SetContext(c)

	tests := []struct {
		name              string
		userSubject       string
		userAccessToken   string
		expectedStatus    int
		expectedPlanCount int
	}{
		{
			name:              "Success with auth",
			userSubject:       "test-subject",
			userAccessToken:   "test-token",
			expectedStatus:    http.StatusOK,
			expectedPlanCount: 3, // startup, pro, enterprise
		},
		{
			name:            "Unauthorized - missing subject",
			userSubject:     "",
			userAccessToken: "test-token",
			expectedStatus:  http.StatusUnauthorized,
		},
		{
			name:            "Unauthorized - missing token",
			userSubject:     "test-subject",
			userAccessToken: "",
			expectedStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/pricing/company", nil)
			req.Header.Set("x-user-subject", tt.userSubject)

			w := httptest.NewRecorder()
			system.GetCompanyPricing(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response struct {
					Prices []Price `json:"prices"`
				}
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Len(t, response.Prices, tt.expectedPlanCount)

				// Verify the structure and content of each plan
				for _, price := range response.Prices {
					assert.NotEmpty(t, price.Title)
					assert.GreaterOrEqual(t, price.TeamMembers, 1)
					assert.GreaterOrEqual(t, price.Projects, 1)
					assert.NotEmpty(t, price.SupportType)

					if price.Title == "Startup" {
						assert.Equal(t, "Most Popular", price.SubTitle)
						assert.Len(t, price.Extras, 1)
						assert.Equal(t, "A/B traffic testing", price.Extras[0].Title)
					}
				}
			}
		})
	}
}

func TestGetGeneralPricing(t *testing.T) {
	c := context.Background()
	testDB, err := setupTestDatabase(c)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() {
		if err := testDB.container.Terminate(c); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	system := setupTestSystem(t)
	system.SetContext(c)

	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pricing/general", nil)
		w := httptest.NewRecorder()

		system.GetGeneralPricing(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Prices []Price `json:"prices"`
		}
		err := json.NewDecoder(w.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Len(t, response.Prices, 4) // all plans including free

		// Verify specific plans exist and have correct attributes
		for _, price := range response.Prices {
			switch price.Title {
			case "free":
				assert.Equal(t, 0, price.Price)
				assert.Equal(t, 1, price.TeamMembers)
				assert.Equal(t, "community", price.SupportType)
			case "startup":
				assert.Equal(t, 29, price.Price)
				assert.Equal(t, 5, price.TeamMembers)
				assert.Equal(t, "email", price.SupportType)
			case "pro":
				assert.Equal(t, 99, price.Price)
				assert.Equal(t, 10, price.TeamMembers)
				assert.Equal(t, "priority", price.SupportType)
			case "enterprise":
				assert.Equal(t, 299, price.Price)
				assert.Equal(t, 50, price.TeamMembers)
				assert.Equal(t, "dedicated", price.SupportType)
			default:
				t.Errorf("Unexpected plan title: %s", price.Title)
			}
		}
	})
}

func TestGetSpecificPrices(t *testing.T) {
	c := context.Background()
	testDB, err := setupTestDatabase(c)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() {
		if err := testDB.container.Terminate(c); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	system := setupTestSystem(t)
	system.SetContext(c)

	tests := []struct {
		name       string
		planType   string
		checkPrice func(*testing.T, Price)
	}{
		{
			name:     "Get Startup Plan",
			planType: "startup",
			checkPrice: func(t *testing.T, price Price) {
				assert.Equal(t, "Startup", price.Title)
				assert.Equal(t, 29, price.Price)
				assert.Equal(t, 5, price.TeamMembers)
				assert.Equal(t, "Most Popular", price.SubTitle)
				assert.Len(t, price.Extras, 1)
				assert.Equal(t, "A/B traffic testing", price.Extras[0].Title)
			},
		},
		{
			name:     "Get Pro Plan",
			planType: "pro",
			checkPrice: func(t *testing.T, price Price) {
				assert.Equal(t, "Pro", price.Title)
				assert.Equal(t, 99, price.Price)
				assert.Equal(t, 10, price.TeamMembers)
				assert.Len(t, price.Extras, 1)
			},
		},
		{
			name:     "Get Enterprise Plan",
			planType: "enterprise",
			checkPrice: func(t *testing.T, price Price) {
				assert.Equal(t, "Enterprise", price.Title)
				assert.Equal(t, 299, price.Price)
				assert.Equal(t, 50, price.TeamMembers)
				assert.Len(t, price.Extras, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var price Price
			switch tt.planType {
			case "startup":
				price = system.GetStartup()
			case "pro":
				price = system.GetPro()
			case "enterprise":
				price = system.GetEnterprise()
			}

			tt.checkPrice(t, price)
		})
	}
}
