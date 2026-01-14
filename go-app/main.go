package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Patient struct {
	ID        int64  `json:"id"`
	Fullname  string `json:"fullname"`
	Sex       string `json:"sex"`
	Birthdate string `json:"birthdate"`
	Address   string `json:"address"`
}

type Response struct {
	Code   int         `json:"code"`
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

type ResponseError struct {
	Code   int    `json:"code"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

type Store struct {
	mu     sync.Mutex
	users  []Patient
	nextID int64
}

func NewStore() *Store {
	return &Store{users: make([]Patient, 0), nextID: 1}
}

func (s *Store) List() Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Patient, len(s.users))
	copy(out, s.users)
	return Response{
		Code:   200,
		Status: "Ok",
		Data:   out,
	}
}

func (s *Store) Get(id int64) (Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.users {
		if p.ID == id {
			return Response{
				Code:   200,
				Status: "Ok",
				Data:   p,
			}, true
		}
	}
	return Response{}, false
}

func (s *Store) Create(p Patient) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = s.nextID
	s.nextID++
	s.users = append(s.users, p)
	return Response{
		Code:   201,
		Status: "Created",
		Data:   p,
	}
}

func (s *Store) Update(id int64, upd Patient) (Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			if upd.Fullname != "" {
				s.users[i].Fullname = upd.Fullname
			}
			if upd.Sex != "" {
				s.users[i].Sex = upd.Sex
			}
			if upd.Birthdate != "" {
				s.users[i].Birthdate = upd.Birthdate
			}
			if upd.Address != "" {
				s.users[i].Address = upd.Address
			}
			return Response{
				Code:   200,
				Status: "Ok",
				Data:   s.users[i],
			}, true
		}
	}
	return Response{}, false
}

func (s *Store) Delete(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return true
		}
	}
	return false
}

// helpers
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func newResponseError(code int, msg string) ResponseError {
	status := http.StatusText(code)
	if status == "" {
		status = "Error"
	}
	return ResponseError{Code: code, Status: status, Error: msg}
}

func writeError(w http.ResponseWriter, respErr ResponseError) {
	if respErr.Code == 0 {
		respErr.Code = http.StatusInternalServerError
	}
	if respErr.Status == "" {
		respErr.Status = http.StatusText(respErr.Code)
	}
	writeJSON(w, respErr.Code, respErr)
}

func main() {
	store := NewStore()
	// seed
	store.Create(Patient{Fullname: "Alice Example", Sex: "F", Birthdate: "1990-01-01", Address: "123 A St"})
	store.Create(Patient{Fullname: "Bob Example", Sex: "M", Birthdate: "1988-05-05", Address: "456 B Ave"})

	mux := http.NewServeMux()

	mux.HandleFunc("/patients", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.List())
			return
		case http.MethodPost:
			var p Patient
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				writeError(w, newResponseError(http.StatusBadRequest, "invalid JSON body"))
				return
			}
			if strings.TrimSpace(p.Fullname) == "" || strings.TrimSpace(p.Sex) == "" || strings.TrimSpace(p.Birthdate) == "" || strings.TrimSpace(p.Address) == "" {
				writeError(w, newResponseError(http.StatusBadRequest, "fullname, sex, birthdate and address are required"))
				return
			}
			created := store.Create(p)
			writeJSON(w, http.StatusCreated, created)
			return
		default:
			writeError(w, newResponseError(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}
	})

	mux.HandleFunc("/patients/", func(w http.ResponseWriter, r *http.Request) {
		// path: /patients/{id}
		idStr := strings.TrimPrefix(r.URL.Path, "/patients/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || idStr == "" {
			writeError(w, newResponseError(http.StatusBadRequest, "invalid id"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			p, ok := store.Get(id)
			if !ok {
				writeError(w, newResponseError(http.StatusNotFound, "patient not found"))
				return
			}
			writeJSON(w, http.StatusOK, p)
			return
		case http.MethodPut:
			var upd Patient
			if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
				writeError(w, newResponseError(http.StatusBadRequest, "invalid JSON body"))
				return
			}
			if strings.TrimSpace(upd.Fullname) == "" && strings.TrimSpace(upd.Sex) == "" && strings.TrimSpace(upd.Birthdate) == "" && strings.TrimSpace(upd.Address) == "" {
				writeError(w, newResponseError(http.StatusBadRequest, "at least one field required to update"))
				return
			}
			updated, ok := store.Update(id, upd)
			if !ok {
				writeError(w, newResponseError(http.StatusNotFound, "patient not found"))
				return
			}
			writeJSON(w, http.StatusOK, updated)
			return
		case http.MethodDelete:
			if ok := store.Delete(id); !ok {
				writeError(w, newResponseError(http.StatusNotFound, "patient not found"))
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			writeError(w, newResponseError(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}
	})

	// serve OpenAPI spec file
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("../openapi.yaml")
		if err != nil {
			writeError(w, newResponseError(http.StatusNotFound, "openapi spec not found"))
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// serve Swagger-like documentation page (uses CDN-hosted swagger-ui)
	mux.HandleFunc("/documentations", func(w http.ResponseWriter, r *http.Request) {
		html := `<!doctype html>
<html>
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		<title>Patients API Docs</title>
		<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@4/swagger-ui.css" />
	</head>
	<body>
		<div id="swagger-ui"></div>
		<script src="https://unpkg.com/swagger-ui-dist@4/swagger-ui-bundle.js"></script>
		<script>
			window.onload = function() {
				SwaggerUIBundle({
					url: '/openapi.yaml',
					dom_id: '#swagger-ui',
				});
			};
		</script>
	</body>
</html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})

	// wrap mux with CORS middleware to allow requests from any origin
	handler := corsMiddleware(mux)

	log.Println("Server listening on :8391")
	log.Fatal(http.ListenAndServe(":8391", handler))
}

// corsMiddleware sets permissive CORS headers and handles preflight requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
