package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/safar/go-sql-store/internal/config"
	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/store"
	"github.com/shopspring/decimal"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}

	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Connect to database: %v", err)
	}
	defer db.Close()

	log.Printf("Connected to database successfully")

	mux := http.NewServeMux()

	mux.HandleFunc("/users", handleUsers(db))
	mux.HandleFunc("/users/", handleUserByID(db))
	mux.HandleFunc("/products", handleProducts(db))
	mux.HandleFunc("/products/", handleProductByID(db))
	mux.HandleFunc("/orders", handleOrders(db))
	mux.HandleFunc("/orders/", handleOrderByID(db))

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		switch r.Method {
		case http.MethodPost:
			var req struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondError(w, http.StatusBadRequest, "Invalid request body")
				return
			}

			user, err := store.CreateUser(ctx, db, req.Email, req.Name)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			respondJSON(w, http.StatusCreated, user)

		case http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
			if pageSize < 1 || pageSize > 100 {
				pageSize = 20
			}

			result, err := store.ListUsers(ctx, db, page, pageSize)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			respondJSON(w, http.StatusOK, result)

		default:
			respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

func handleUserByID(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		idStr := r.URL.Path[len("/users/"):]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		user, err := store.GetUser(ctx, db, id)
		if err != nil {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, user)
	}
}

func handleProducts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		switch r.Method {
		case http.MethodPost:
			var req struct {
				SKU         string  `json:"sku"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Price       float64 `json:"price"`
				Stock       int     `json:"stock"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondError(w, http.StatusBadRequest, "Invalid request body")
				return
			}

			price := decimal.NewFromFloat(req.Price)
			product, err := store.CreateProduct(ctx, db, req.SKU, req.Name, req.Description, price, req.Stock)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			respondJSON(w, http.StatusCreated, product)

		case http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
			if pageSize < 1 || pageSize > 100 {
				pageSize = 20
			}

			result, err := store.ListProducts(ctx, db, page, pageSize)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			respondJSON(w, http.StatusOK, result)

		default:
			respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

func handleProductByID(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		idStr := r.URL.Path[len("/products/"):]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid product ID")
			return
		}

		product, err := store.GetProduct(ctx, db, id)
		if err != nil {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, product)
	}
}

func handleOrders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		switch r.Method {
		case http.MethodPost:
			var req struct {
				UserID int64 `json:"user_id"`
				Items  []struct {
					ProductID int64 `json:"product_id"`
					Quantity  int   `json:"quantity"`
				} `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondError(w, http.StatusBadRequest, "Invalid request body")
				return
			}

			var items []store.OrderItemRequest
			for _, item := range req.Items {
				items = append(items, store.OrderItemRequest{
					ProductID: item.ProductID,
					Quantity:  item.Quantity,
				})
			}

			order, err := store.CreateOrder(ctx, db, store.CreateOrderRequest{
				UserID: req.UserID,
				Items:  items,
			})
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			respondJSON(w, http.StatusCreated, order)

		default:
			respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

func handleOrderByID(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		idStr := r.URL.Path[len("/orders/"):]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid order ID")
			return
		}

		order, err := store.GetOrder(ctx, db, id)
		if err != nil {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, order)
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
