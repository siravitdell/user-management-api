package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	userv1 "github.com/siravitdell/user-management-api/gen/user/v1"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	token := flag.String("token", os.Getenv("TOKEN"), "JWT access token (or set $TOKEN)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: grpc-client [-addr host:port] [-token JWT] <create|get> [flags]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	client := userv1.NewUserServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*token)

	args := flag.Args()[1:]
	switch flag.Arg(0) {
	case "create":
		fs := flag.NewFlagSet("create", flag.ExitOnError)
		name := fs.String("name", "", "user name")
		email := fs.String("email", "", "user email")
		password := fs.String("password", "", "user password")
		_ = fs.Parse(args)
		resp, err := client.CreateUser(ctx, &userv1.CreateUserRequest{Name: *name, Email: *email, Password: *password})
		printResult(resp, err)
	case "get":
		fs := flag.NewFlagSet("get", flag.ExitOnError)
		id := fs.String("id", "", "user id")
		_ = fs.Parse(args)
		resp, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: *id})
		printResult(resp, err)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func printResult(resp proto.Message, err error) {
	if err != nil {
		fail(err)
	}
	out, _ := protojson.MarshalOptions{Multiline: true}.Marshal(resp)
	fmt.Println(string(out))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
