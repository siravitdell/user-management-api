package grpcapi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	userv1 "github.com/siravitdell/user-management-api/gen/user/v1"
	"github.com/siravitdell/user-management-api/internal/adapter/outbound/security"
	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
	"github.com/siravitdell/user-management-api/internal/core/service"
)

// fakeRepo implements only what CreateUser/GetUser need; any other method
// panics via the embedded nil interface.
type fakeRepo struct {
	port.UserRepository
	mu    sync.Mutex
	users map[string]domain.User
}

func (r *fakeRepo) Create(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if existing.Email == u.Email {
			return domain.ErrEmailAlreadyExists
		}
	}
	u.ID = strconv.Itoa(len(r.users) + 1)
	r.users[u.ID] = *u
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return &u, nil
}

func startServer(t *testing.T) (userv1.UserServiceClient, port.TokenManager) {
	t.Helper()
	repo := &fakeRepo{users: map[string]domain.User{}}
	hasher, _ := security.NewBcryptHasher(bcrypt.MinCost)
	tokens, _ := security.NewJWTManager("grpc-test-secret-grpc-test-secret!!", "user-api", time.Hour)
	users := service.NewUserService(repo, hasher)
	auth, err := service.NewAuthService(repo, hasher, tokens)
	if err != nil {
		t.Fatal(err)
	}

	lis := bufconn.Listen(1 << 20)
	srv := NewGRPCServer(users, auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return userv1.NewUserServiceClient(conn), tokens
}

func authed(t *testing.T, tokens port.TokenManager) context.Context {
	t.Helper()
	tok, err := tokens.Generate("caller", "caller@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok.AccessToken)
}

func TestGRPC_CreateAndGetUser(t *testing.T) {
	client, tokens := startServer(t)
	ctx := authed(t, tokens)

	created, err := client.CreateUser(ctx, &userv1.CreateUserRequest{Name: "Dana", Email: "Dana@Example.com", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	u := created.GetUser()
	if u.GetId() == "" || u.GetEmail() != "dana@example.com" || u.GetCreatedAt().AsTime().IsZero() {
		t.Fatalf("unexpected user %+v", u)
	}

	got, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: u.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetUser().GetName() != "Dana" {
		t.Fatalf("unexpected %+v", got.GetUser())
	}
}

func TestGRPC_ErrorCodes(t *testing.T) {
	client, tokens := startServer(t)
	ctx := authed(t, tokens)

	_, _ = client.CreateUser(ctx, &userv1.CreateUserRequest{Name: "E", Email: "e@example.com", Password: "password123"})

	cases := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"duplicate", func() error {
			_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{Name: "E", Email: "e@example.com", Password: "password123"})
			return err
		}, codes.AlreadyExists},
		{"validation", func() error {
			_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{Email: "bad"})
			return err
		}, codes.InvalidArgument},
		{"not found", func() error {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: "missing"})
			return err
		}, codes.NotFound},
		{"empty id", func() error {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{})
			return err
		}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := status.Code(tc.call()); got != tc.want {
				t.Fatalf("code = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGRPC_RequiresToken(t *testing.T) {
	client, _ := startServer(t)

	for name, ctx := range map[string]context.Context{
		"no metadata":  context.Background(),
		"wrong scheme": metadata.AppendToOutgoingContext(context.Background(), "authorization", "Basic abc"),
		"bad token":    metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer nope"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: "x"})
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
			}
		})
	}
}
