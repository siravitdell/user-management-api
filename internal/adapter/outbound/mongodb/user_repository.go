package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

const CollectionName = "users"

type userDocument struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Name      string        `bson:"name"`
	Email     string        `bson:"email"`
	Password  string        `bson:"password"`
	CreatedAt time.Time     `bson:"created_at"`
}

func toDocument(u *domain.User) userDocument {
	return userDocument{Name: u.Name, Email: u.Email, Password: u.PasswordHash, CreatedAt: u.CreatedAt}
}

func (d userDocument) toDomain() *domain.User {
	return &domain.User{
		ID:           d.ID.Hex(),
		Name:         d.Name,
		Email:        d.Email,
		PasswordHash: d.Password,
		CreatedAt:    d.CreatedAt.UTC(),
	}
}

func Connect(ctx context.Context, uri string) (*mongo.Client, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client, nil
}

type UserRepository struct {
	coll *mongo.Collection
}

var _ port.UserRepository = (*UserRepository)(nil)

func NewUserRepository(ctx context.Context, db *mongo.Database) (*UserRepository, error) {
	r := &UserRepository{coll: db.Collection(CollectionName)}
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_email"),
	})
	if err != nil {
		return nil, fmt.Errorf("create email index: %w", err)
	}
	return r, nil
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	res, err := r.coll.InsertOne(ctx, toDocument(u))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrEmailAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	oid, ok := res.InsertedID.(bson.ObjectID)
	if !ok {
		return fmt.Errorf("unexpected inserted id type %T", res.InsertedID)
	}
	u.ID = oid.Hex()
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}
	return r.findOne(ctx, bson.D{{Key: "_id", Value: oid}})
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.D) (*domain.User, error) {
	var doc userDocument
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	cur, err := r.coll.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer cur.Close(ctx)

	var docs []userDocument
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}
	users := make([]domain.User, 0, len(docs))
	for _, d := range docs {
		users = append(users, *d.toDomain())
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, id string, upd domain.UserUpdate) (*domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}

	set := bson.D{}
	if upd.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *upd.Name})
	}
	if upd.Email != nil {
		set = append(set, bson.E{Key: "email", Value: *upd.Email})
	}
	if len(set) == 0 {
		return r.GetByID(ctx, id)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var doc userDocument
	err = r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: oid}}, bson.D{{Key: "$set", Value: set}}, opts).Decode(&doc)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return nil, domain.ErrUserNotFound
	case mongo.IsDuplicateKeyError(err):
		return nil, domain.ErrEmailAlreadyExists
	case err != nil:
		return nil, fmt.Errorf("update user: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrUserNotFound
	}
	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	n, err := r.coll.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}
