package user

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Nerzal/gocloak/v13"
	"github.com/bugfixes/go-bugfixes/logs"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strings"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}

func (s *System) ValidateUser(ctx context.Context, subject string) bool {
	if subject == "" {
		return false
	}

	user, err := s.GetKeycloakDetails(ctx, subject)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return false
		}

		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak details: %v", err)
		return false
	}
	if user == nil {
		return false
	}

	return true
}

func (s *System) GetKeycloakDetails(ctx context.Context, subject string) (*gocloak.User, error) {
	client, token, err := s.Config.Keycloak.GetClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	user, err := client.GetUserByID(ctx, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get user by id: %v", err)
	}

	return user, nil
}

func (s *System) CreateUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	userSubject := r.Header.Get("x-user-subject")
	if userSubject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No user subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cloakDetails, err := s.GetKeycloakDetails(s.Context, userSubject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := s.RetrieveUserDetails(userSubject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// User already exists
	if user != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Create user
	if err := s.CreateUserDetails(userSubject, *cloakDetails.Email, *cloakDetails.FirstName, *cloakDetails.LastName); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := s.RetrieveUserDetails(subject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(user); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	type formData struct {
		KnownAs string `json:"knownAs"`
		Email   string `json:"emailAddress"`
	}
	fd := formData{}
	if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode form data: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logs.Logf("knownAs: %s, email: %s, subject: %s", fd.KnownAs, fd.Email, subject)
}

func (s *System) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	n := &Notifications{}

	notifications, err := s.RetrieveUserNotifications(subject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if notifications == nil {
		if err := json.NewEncoder(w).Encode(n); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user notifications: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		return
	}

	n.Notifications = notifications
	if err := json.NewEncoder(w).Encode(n); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUserNotification(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.MarkNotificationAsRead(subject, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUserNotification(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.DeleteUserNotificationInDB(subject, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UploadThing(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	type Files struct {
		Name     string `json:"name"`
		Size     int    `json:"size"`
		Type     string `json:"type"`
		CustomID string `json:"customId"`
	}
	type fileCreate struct {
		Files              []Files `json:"files"`
		ACL                string  `json:"acl"`
		ContentDisposition string  `json:"contentDisposition"`
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to parse form: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	clientFiles := r.MultipartForm.File["files"]
	var files []Files
	for _, file := range clientFiles {
		files = append(files, Files{
			Name: file.Filename,
			Size: int(file.Size),
			Type: file.Header.Get("Content-Type"),
		})
	}
	fc := fileCreate{
		Files:              files,
		ACL:                "public-read",
		ContentDisposition: "inline",
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(fc); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to marshal request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	uploadThing := "https://uploadthing.com/api/uploadFiles"
	req, err := http.NewRequest(http.MethodPost, uploadThing, buf)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-uploadthing-api-key", s.Config.Local.GetValue("UPLOADTHING_SECRET"))
	req.Header.Set("x-uploadthing-version", "6.4.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to upload thing: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close body: %v", err)
		}
	}()

	var bd interface{}
	if err := json.NewDecoder(resp.Body).Decode(&bd); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = fmt.Sprintf("Response: %v", bd)

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUserImage(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	s.Context = r.Context()

	userId := r.Header.Get("x-user-subject")

	imageChange := struct {
		Image string `json:"image"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&imageChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.UpdateUserImageInDB(userId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logs.Logf("Deleting user: %s", subject)

	//if err := s.DeleteUserInDB(subject); err != nil {
	//	_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete user: %v", err)
	//	w.WriteHeader(http.StatusInternalServerError)
	//	return
	//}

	w.WriteHeader(http.StatusOK)
}
