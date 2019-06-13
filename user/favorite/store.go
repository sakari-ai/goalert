package favorite

import (
	"context"
	"database/sql"
	"github.com/target/goalert/assignment"
	"github.com/target/goalert/permission"
	"github.com/target/goalert/util"
	"github.com/target/goalert/validation/validate"

	"github.com/pkg/errors"
)

// Store allows the lookup and management of Favorites.
type Store interface {
	// Set will set a target as a favorite for the given userID. It is safe to call multiple times.
	Set(ctx context.Context, userID string, tgt assignment.Target) error
	// Unset will unset a target as a favorite for the given userID. It is safe to call multiple times.
	Unset(ctx context.Context, userID string, tgt assignment.Target) error

	FindAll(ctx context.Context, userID string, filter []assignment.TargetType) ([]assignment.Target, error)
}

// DB implements the Store interface using a postgres database.
type DB struct {
	db *sql.DB

	insert  *sql.Stmt
	delete  *sql.Stmt
	findAll *sql.Stmt
}

// NewDB will create a DB backend from a sql.DB. An error will be returned if statements fail to prepare.
func NewDB(ctx context.Context, db *sql.DB) (*DB, error) {
	p := &util.Prepare{DB: db, Ctx: ctx}
	return &DB{
		db: db,
		insert: p.P(`
			INSERT INTO user_favorites (user_id, tgt_service_id, tgt_rotation_id)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING
		`),
		delete: p.P(`
			DELETE FROM user_favorites
			WHERE user_id = $1 and
			tgt_service_id  = $2 OR tgt_rotation_id = $3
			
		`),
		findAll: p.P(`
			SELECT tgt_service_id
			FROM user_favorites
			WHERE user_id = $1 
			AND tgt_service_id NOTNULL = $2
		`),
	}, p.Err
}

// Set will store the target as a favorite of the given user. Must be authorized as System or the same user.
func (db *DB) Set(ctx context.Context, userID string, tgt assignment.Target) error {
	err := permission.LimitCheckAny(ctx, permission.System, permission.MatchUser(userID))
	if err != nil {
		return err
	}
	err = validate.Many(
		validate.UUID("TargetID", tgt.TargetID()),
		validate.UUID("UserID", userID),
		validate.OneOf("TargetType", tgt.TargetType(), assignment.TargetTypeService,  assignment.TargetTypeRotation, assignment.TargetTypeSchedule),
	)
	if err != nil {
		return err
	}

	var rotationID, serviceID sql.NullString



	switch tgt.TargetType() {
	case assignment.TargetTypeRotation:
		rotationID.Valid = true
		rotationID.String = tgt.TargetID()

	case assignment.TargetTypeService:
		serviceID.Valid = true
		serviceID.String = tgt.TargetID()

	}

	_, err = db.insert.ExecContext(ctx, userID, serviceID, rotationID)
	if err != nil {
		return errors.Wrap(err, "set favorite")
	}





	return nil
}

// Unset will remove the target as a favorite of the given user. Must be authorized as System or the same user.
func (db *DB) Unset(ctx context.Context, userID string, tgt assignment.Target) error {
	err := permission.LimitCheckAny(ctx, permission.System, permission.MatchUser(userID))
	if err != nil {
		return err
	}



	err = validate.Many(
		validate.UUID("TargetID", tgt.TargetID()),
		validate.UUID("UserID", userID),
		validate.OneOf("TargetType", tgt.TargetType(),  assignment.TargetTypeService, assignment.TargetTypeRotation, assignment.TargetTypeSchedule),
	)
	if err != nil {
		return err
	}

	var rotationID, serviceID sql.NullString


	switch tgt.TargetType() {
	case assignment.TargetTypeRotation:
		rotationID.Valid = true
		rotationID.String = tgt.TargetID()

	case assignment.TargetTypeService:
		serviceID.Valid = true
		serviceID.String = tgt.TargetID()

	}


	_, err = db.delete.ExecContext(ctx, userID, serviceID, rotationID)
	if err == sql.ErrNoRows {
		// ignoring since it is safe to unset favorite (with retries)
		err = nil
	}
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) FindAll(ctx context.Context, userID string, filter []assignment.TargetType) ([]assignment.Target, error) {
	err := permission.LimitCheckAny(ctx, permission.System, permission.MatchUser(userID))
	if err != nil {
		return nil, err
	}

	err = validate.Many(
		validate.UUID("UserID", userID),
		validate.Range("Filter", len(filter), 0, 50),
	)
	if err != nil {
		return nil, err
	}

	var allowServices bool
	if len(filter) == 0 {
		allowServices = true
	} else {
		for _, f := range filter {
			switch f {
			case assignment.TargetTypeService:
				allowServices = true
			case assignment.TargetTypeRotation:
				allowServices = true
			}
		}
	}

	rows, err := db.findAll.QueryContext(ctx, userID, allowServices)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []assignment.Target

	for rows.Next() {
		var svc sql.NullString
		err = rows.Scan(&svc)
		if err != nil {
			return nil, err
		}
		switch {
		case svc.Valid:
			targets = append(targets, assignment.ServiceTarget(svc.String))
		}
	}
	return targets, nil
}
