package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/app"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/security"
)

type principalKey struct{}

type Config struct {
	HandlerTimeout   time.Duration
	RequestBodyLimit int64
	DefaultPageSize  int
	MaxPageSize      int
}

type API struct {
	service app.UseCases
	jwt     *security.JWT
	config  Config
}

func New(service app.UseCases, jwt *security.JWT, config Config) http.Handler {
	a := &API{service: service, jwt: jwt, config: config}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(config.HandlerTimeout))
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Post("/login", a.login)
	r.Group(func(r chi.Router) {
		r.Use(a.authenticate)
		r.Route("/api/users", func(r chi.Router) {
			r.Post("/", a.createUser)
			r.Get("/", a.findUsers)
			r.Get("/{id}", a.getUser)
			r.Get("/{id}/accounts", a.getUserAccounts)
			r.Get("/{id}/friends", a.getFriends)
			r.Post("/{id}/friends", a.addFriends)
			r.Delete("/{id}/friends", a.deleteFriends)
		})
		r.Route("/api/accounts", func(r chi.Router) {
			r.Post("/", a.createAccount)
			r.Get("/", a.findAccounts)
			r.Get("/{id}", a.getAccount)
			r.Post("/{id}/deposit", a.deposit)
			r.Post("/{id}/withdraw", a.withdraw)
			r.Post("/{id}/transfer", a.transfer)
			r.Get("/{id}/balance", a.balanceInCurrency)
		})
		r.Get("/api/transfers", a.findTransfers)
		r.Get("/api/balance-operations", a.findBalanceOperations)
	})
	return r
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.Login) == "" || strings.TrimSpace(body.Password) == "" {
		writeError(w, domain.NewError(domain.ErrValidation, "login and password must not be blank"))
		return
	}
	user, err := a.service.Authenticate(r.Context(), body.Login, body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	token, err := a.jwt.Generate(user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleAdmin) {
		return
	}
	var body struct {
		Login, FirstName, LastName, Password string
		Age                                  int           `json:"age"`
		Gender                               domain.Gender `json:"gender"`
		Role                                 domain.Role   `json:"role"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	user, err := a.service.CreateUser(r.Context(), app.CreateUserRequest{Login: body.Login, FirstName: body.FirstName, LastName: body.LastName, Password: body.Password, Age: body.Age, Gender: body.Gender, Role: body.Role})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := principal(r)
	if p.Role != domain.RoleAdmin && p.UserID != id {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
		return
	}
	user, err := a.service.User(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) findUsers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleAdmin) {
		return
	}
	page, pageSize, err := a.pagination(r)
	if err != nil {
		writeError(w, err)
		return
	}
	minAge, err := optionalInt(r.URL.Query().Get("minAge"))
	if err != nil {
		writeError(w, err)
		return
	}
	maxAge, err := optionalInt(r.URL.Query().Get("maxAge"))
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.Users(r.Context(), app.UserFilter{Login: r.URL.Query().Get("login"), FirstName: r.URL.Query().Get("firstName"), LastName: r.URL.Query().Get("lastName"), MinAge: minAge, MaxAge: maxAge, Genders: queryList(r, "genders"), Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) friendsAction(w http.ResponseWriter, r *http.Request, add bool) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if principal(r).UserID != id {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
		return
	}
	var body struct {
		FriendIDs []uuid.UUID `json:"friendIds"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if len(body.FriendIDs) == 0 {
		writeError(w, domain.NewError(domain.ErrValidation, "friendIds must not be empty"))
		return
	}
	var err error
	if add {
		err = a.service.AddFriends(r.Context(), id, body.FriendIDs)
	} else {
		err = a.service.DeleteFriends(r.Context(), id, body.FriendIDs)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) addFriends(w http.ResponseWriter, r *http.Request)    { a.friendsAction(w, r, true) }
func (a *API) deleteFriends(w http.ResponseWriter, r *http.Request) { a.friendsAction(w, r, false) }

func (a *API) getFriends(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := principal(r)
	if p.Role != domain.RoleAdmin && p.UserID != id {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
		return
	}
	items, err := a.service.Friends(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) getUserAccounts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := principal(r)
	if p.Role != domain.RoleAdmin && p.UserID != id {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
		return
	}
	items, err := a.service.UserAccounts(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	var body struct {
		Balance decimal.Decimal `json:"balance"`
		OwnerID uuid.UUID       `json:"ownerId"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if principal(r).UserID != body.OwnerID {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "ownerId must match current user"))
		return
	}
	account, err := a.service.CreateAccount(r.Context(), body.OwnerID, body.Balance)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	account, err := a.service.Account(r.Context(), id, principal(r).UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (a *API) findAccounts(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	page, pageSize, err := a.pagination(r)
	if err != nil {
		writeError(w, err)
		return
	}
	min, err := optionalDecimal(r.URL.Query().Get("minBalance"))
	if err != nil {
		writeError(w, err)
		return
	}
	max, err := optionalDecimal(r.URL.Query().Get("maxBalance"))
	if err != nil {
		writeError(w, err)
		return
	}
	owner, err := optionalUUID(r.URL.Query().Get("ownerId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if p.Role != domain.RoleAdmin {
		if owner != nil && *owner != p.UserID {
			writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
			return
		}
		owner = &p.UserID
	}
	result, err := a.service.Accounts(r.Context(), app.AccountFilter{MinBalance: min, MaxBalance: max, OwnerID: owner, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) amountAction(w http.ResponseWriter, r *http.Request, deposit bool) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Amount decimal.Decimal `json:"amount"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	var result domain.BalanceOperationDetailed
	var err error
	if deposit {
		result, err = a.service.Deposit(r.Context(), id, principal(r).UserID, body.Amount)
	} else {
		result, err = a.service.Withdraw(r.Context(), id, principal(r).UserID, body.Amount)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) deposit(w http.ResponseWriter, r *http.Request)  { a.amountAction(w, r, true) }
func (a *API) withdraw(w http.ResponseWriter, r *http.Request) { a.amountAction(w, r, false) }

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		ToAccountID uuid.UUID       `json:"toAccountId"`
		Amount      decimal.Decimal `json:"amount"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.Transfer(r.Context(), id, body.ToAccountID, principal(r).UserID, body.Amount)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) balanceInCurrency(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, domain.RoleUser) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	result, err := a.service.BalanceInCurrency(r.Context(), id, principal(r).UserID, r.URL.Query().Get("currency"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) findBalanceOperations(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	page, pageSize, err := a.pagination(r)
	if err != nil {
		writeError(w, err)
		return
	}
	accountID, err := optionalUUID(r.URL.Query().Get("accountId"))
	if err != nil {
		writeError(w, err)
		return
	}
	min, err := optionalDecimal(r.URL.Query().Get("minAmount"))
	if err != nil {
		writeError(w, err)
		return
	}
	max, err := optionalDecimal(r.URL.Query().Get("maxAmount"))
	if err != nil {
		writeError(w, err)
		return
	}
	minDate, err := optionalTime(r.URL.Query().Get("minDate"))
	if err != nil {
		writeError(w, err)
		return
	}
	maxDate, err := optionalTime(r.URL.Query().Get("maxDate"))
	if err != nil {
		writeError(w, err)
		return
	}
	var owner *uuid.UUID
	if p.Role == domain.RoleUser {
		owner = &p.UserID
	}
	result, err := a.service.BalanceOperations(r.Context(), app.OperationFilter{AccountID: accountID, Types: queryList(r, "types"), MinAmount: min, MaxAmount: max, MinDate: minDate, MaxDate: maxDate, OwnerID: owner, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) findTransfers(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	page, pageSize, err := a.pagination(r)
	if err != nil {
		writeError(w, err)
		return
	}
	sender, err := optionalUUID(r.URL.Query().Get("senderAccountId"))
	if err != nil {
		writeError(w, err)
		return
	}
	receiver, err := optionalUUID(r.URL.Query().Get("receiverAccountId"))
	if err != nil {
		writeError(w, err)
		return
	}
	min, err := optionalDecimal(r.URL.Query().Get("minAmount"))
	if err != nil {
		writeError(w, err)
		return
	}
	max, err := optionalDecimal(r.URL.Query().Get("maxAmount"))
	if err != nil {
		writeError(w, err)
		return
	}
	minDate, err := optionalTime(r.URL.Query().Get("minDate"))
	if err != nil {
		writeError(w, err)
		return
	}
	maxDate, err := optionalTime(r.URL.Query().Get("maxDate"))
	if err != nil {
		writeError(w, err)
		return
	}
	var owner *uuid.UUID
	if p.Role == domain.RoleUser {
		owner = &p.UserID
	}
	result, err := a.service.Transfers(r.Context(), app.TransferFilter{SenderID: sender, ReceiverID: receiver, MinAmount: min, MaxAmount: max, MinDate: minDate, MaxDate: maxDate, OwnerID: owner, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(raw, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Code: "UNAUTHORIZED", Message: "Authentication required"})
			return
		}
		claims, err := a.jwt.Parse(strings.TrimSpace(strings.TrimPrefix(raw, "Bearer ")))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Code: "UNAUTHORIZED", Message: "Authentication required"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, claims)))
	})
}

func principal(r *http.Request) security.Claims {
	return r.Context().Value(principalKey{}).(security.Claims)
}
func requireRole(w http.ResponseWriter, r *http.Request, role domain.Role) bool {
	if principal(r).Role != role {
		writeError(w, domain.NewError(domain.ErrAccessDenied, "access denied"))
		return false
	}
	return true
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "SERVICE_ERROR"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "RESOURCE_NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "RESOURCE_CONFLICT"
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "VALIDATION_ERROR"
	case errors.Is(err, domain.ErrNotEnoughFunds):
		status, code = http.StatusBadRequest, "NOT_ENOUGH_FUNDS"
	case errors.Is(err, domain.ErrAccessDenied):
		status, code = http.StatusForbidden, "ACCESS_DENIED"
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, code = http.StatusUnauthorized, "INVALID_CREDENTIALS"
	case errors.Is(err, domain.ErrExternalTimeout):
		status, code = http.StatusGatewayTimeout, "EXTERNAL_SERVICE_TIMEOUT"
	default:
		slog.Error("request failed", "error", err)
		err = fmt.Errorf("internal server error")
	}
	writeJSON(w, status, errorResponse{Code: code, Message: err.Error()})
}

func (a *API) decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, a.config.RequestBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.ErrValidation, "request body is invalid")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, domain.NewError(domain.ErrValidation, "id must be a UUID"))
		return uuid.Nil, false
	}
	return id, true
}

func (a *API) pagination(r *http.Request) (int, int, error) {
	page, size := 0, a.config.DefaultPageSize
	var err error
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 0 {
			return 0, 0, domain.NewError(domain.ErrValidation, "page must be a non-negative integer")
		}
	}
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil || size < 1 || size > a.config.MaxPageSize {
			return 0, 0, domain.NewError(domain.ErrValidation, fmt.Sprintf("pageSize must be between 1 and %d", a.config.MaxPageSize))
		}
	}
	return page, size, nil
}
func optionalInt(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil, domain.NewError(domain.ErrValidation, "invalid integer filter")
	}
	return &value, nil
}
func optionalUUID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := uuid.Parse(raw)
	if err != nil {
		return nil, domain.NewError(domain.ErrValidation, "invalid UUID filter")
	}
	return &value, nil
}
func optionalDecimal(raw string) (*decimal.Decimal, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() {
		return nil, domain.NewError(domain.ErrValidation, "invalid amount filter")
	}
	return &value, nil
}
func optionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, domain.NewError(domain.ErrValidation, "date filters must use RFC3339")
	}
	return &value, nil
}
func queryList(r *http.Request, key string) []string {
	values := r.URL.Query()[key]
	if len(values) == 1 {
		values = strings.Split(values[0], ",")
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.ToUpper(strings.TrimSpace(value)); value != "" {
			out = append(out, value)
		}
	}
	return out
}
