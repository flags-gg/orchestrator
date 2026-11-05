package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/auth"
	ConfigBuilder "github.com/keloran/go-config"
)

var getClerkUser = clerkUser.Get

type loginProfile struct {
	knownAs, email, first, last string
}

func loadLoginProfile(ctx context.Context, subject string) (loginProfile, error) {
	usr, err := getClerkUser(ctx, subject)
	if err != nil {
		return loginProfile{}, err
	}
	if usr == nil {
		return loginProfile{}, errors.New("Clerk returned no user")
	}
	return profileFromClerk(usr)
}

func profileFromClerk(usr *clerk.User) (loginProfile, error) {
	p := loginProfile{}
	if usr.FirstName != nil {
		p.first = *usr.FirstName
	}
	if usr.LastName != nil {
		p.last = *usr.LastName
	}
	for _, email := range usr.EmailAddresses {
		if email != nil && usr.PrimaryEmailAddressID != nil && email.ID == *usr.PrimaryEmailAddressID {
			p.email = email.EmailAddress
			break
		}
	}
	if p.email == "" {
		return p, errors.New("Clerk user has no primary email")
	}
	if usr.Username != nil {
		p.knownAs = strings.TrimSpace(*usr.Username)
	}
	if p.knownAs == "" {
		p.knownAs = strings.TrimSpace(p.first + " " + p.last)
	}
	if p.knownAs == "" {
		p.knownAs = strings.SplitN(p.email, "@", 2)[0]
	}
	return p, nil
}

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type formData struct {
		KnownAs   string `json:"knownAs"`
		Email     string `json:"emailAddress"`
		First     string `json:"firstName"`
		Last      string `json:"lastName"`
		UserGroup int    `json:"userGroup"`
		Location  string `json:"location"`
	}
	fd := formData{}
	if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode form data: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if fd.KnownAs == "" {
		profile, err := loadLoginProfile(ctx, userId)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fd.KnownAs, fd.First, fd.Last, fd.Email = profile.knownAs, profile.first, profile.last, profile.email
	}

	if fd.Location == "" {
		fd.Location = "Unknown"
	}

	// Create user
	if err := s.CreateUserDetails(ctx, userId, fd.KnownAs, fd.Email, fd.First, fd.Last, fd.Location, 1); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := &User{}

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	dbuser, err := s.RetrieveUserDetailsDB(ctx, userId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dbuser == nil {
		profile, err := loadLoginProfile(ctx, userId)
		if err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to load Clerk profile for first login: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if err := s.createUserIfMissing(ctx, userId, profile.knownAs, profile.email, profile.first, profile.last); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to provision user on first login: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		dbuser, err = s.RetrieveUserDetailsDB(ctx, userId)
		if err != nil || dbuser == nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve provisioned user: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	user = dbuser
	user.Created = true

	buf, err := json.Marshal(user)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to write response: %v", err)
	}
}

func (s *System) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	type formData struct {
		KnownAs   string `json:"knownAs"`
		Email     string `json:"emailAddress"`
		First     string `json:"firstName"`
		Last      string `json:"lastName"`
		UserGroup int    `json:"userGroup"`
		Location  string `json:"location"`
	}
	fd := formData{}
	if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode form data: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.UpdateUserDetailsDB(ctx, userId, fd.KnownAs, fd.Email, fd.First, fd.Last, fd.Location); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	n := &Notifications{}

	notifications, err := s.RetrieveUserNotifications(ctx, userId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if notifications != nil {
		n.Notifications = notifications
	}

	buf, err := json.Marshal(n)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to write response: %v", err)
	}
}

func (s *System) UpdateUserNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.MarkNotificationAsRead(ctx, userId, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUserNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.DeleteUserNotificationInDB(ctx, userId, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UploadThing(w http.ResponseWriter, r *http.Request) {
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

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUserImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	imageChange := struct {
		Image string `json:"image"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&imageChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.UpdateUserImageInDB(ctx, userId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := s.DeleteUserInDB(ctx, userId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
