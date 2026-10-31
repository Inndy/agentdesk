package desk

import (
	"database/sql"
	"errors"
	"time"
)

// NewLoginLink creates a separate, one-use credential for a Telegram issue URL.
// Only its hash is persisted; the long-lived human credential is never sent.
func (s *Store) NewLoginLink(issueID string) (string, error) {
	token := RandomToken()
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM login_links WHERE expires<=?", time.Now().Unix()); err != nil {
		return "", err
	}
	if _, err = tx.Exec("INSERT INTO login_links(hash,issue_id,expires) VALUES(?,?,?)", digest(token), issueID, time.Now().Add(24*time.Hour).Unix()); err != nil {
		return "", err
	}
	return token, tx.Commit()
}

// RedeemLoginLink consumes the link and creates the session in one transaction.
func (s *Store) RedeemLoginLink(token string) (string, error) {
	if len(token) != 64 {
		return "", ErrNotFound
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var issueID string
	err = tx.QueryRow("DELETE FROM login_links WHERE hash=? AND expires>? RETURNING issue_id", digest(token), time.Now().Unix()).Scan(&issueID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	session := RandomToken()
	if _, err = tx.Exec("DELETE FROM sessions WHERE expires<=?", time.Now().Unix()); err != nil {
		return "", err
	}
	if _, err = tx.Exec("INSERT INTO sessions(hash,expires) VALUES(?,?)", digest(session), time.Now().Add(7*24*time.Hour).Unix()); err != nil {
		return "", err
	}
	return session, tx.Commit()
}
