package user

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	ConfigBuilder "github.com/keloran/go-config"
)

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

	userSubject := r.Header.Get("x-user-subject")
	if userSubject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No user subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
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
		fd.KnownAs = *usr.Username
		fd.First = *usr.FirstName
		fd.Last = *usr.LastName
		fd.Email = usr.EmailAddresses[0].EmailAddress
	}

	if fd.Location == "" {
		fd.Location = "Unknown"
	}

	// Create user
	if err := s.CreateUserDetails(ctx, userSubject, fd.KnownAs, fd.Email, fd.First, fd.Last, fd.Location, 1); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := &User{}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	dbuser, err := s.RetrieveUserDetailsDB(ctx, usr.ID)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dbuser != nil {
		user = dbuser
		user.Created = true
	} else {
		user.Id = &usr.ID
		user.Email = &usr.EmailAddresses[0].EmailAddress
		user.FirstName = usr.FirstName
		user.LastName = usr.LastName
	}

	if err := json.NewEncoder(w).Encode(user); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	subject := r.Header.Get("x-user-subject")
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

	if err := s.UpdateUserDetailsDB(ctx, subject, fd.KnownAs, fd.Email, fd.First, fd.Last, fd.Location); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	n := &Notifications{}

	notifications, err := s.RetrieveUserNotifications(ctx, usr.ID)
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
	ctx := r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.MarkNotificationAsRead(ctx, usr.ID, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUserNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.DeleteUserNotificationInDB(ctx, usr.ID, notificationId); err != nil {
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
	_ = fmt.Sprintf("Response: %v", bd)

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUserImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
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

	if err := s.UpdateUserImageInDB(ctx, usr.ID, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("Missing x-user-subject header")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := s.DeleteUserInDB(ctx, usr.ID); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
