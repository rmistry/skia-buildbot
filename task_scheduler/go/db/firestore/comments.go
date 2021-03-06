package firestore

import (
	"fmt"
	"net/url"
	"time"

	fs "cloud.google.com/go/firestore"
	"go.skia.org/infra/go/firestore"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/task_scheduler/go/db"
	"go.skia.org/infra/task_scheduler/go/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	COLLECTION_COMMIT_COMMENTS    = "commit-comments"
	COLLECTION_TASK_COMMENTS      = "task-comments"
	COLLECTION_TASK_SPEC_COMMENTS = "task-spec-comments"
)

// commitComments returns a reference to the commit comments collection.
func (d *firestoreDB) commitComments() *fs.CollectionRef {
	return d.client.Collection(COLLECTION_COMMIT_COMMENTS)
}

// taskComments returns a reference to the task comments collection.
func (d *firestoreDB) taskComments() *fs.CollectionRef {
	return d.client.Collection(COLLECTION_TASK_COMMENTS)
}

// taskSpecComments returns a reference to the task spec comments collection.
func (d *firestoreDB) taskSpecComments() *fs.CollectionRef {
	return d.client.Collection(COLLECTION_TASK_SPEC_COMMENTS)
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) GetCommentsForRepos(repos []string, from time.Time) ([]*types.RepoComments, error) {
	from = firestore.FixTimestamp(from)
	// TODO(borenet): We should come up with something more efficient, but
	// this is convenient because it doesn't require composite indexes.
	commentsByRepo := make(map[string]*types.RepoComments, len(repos))
	for _, repo := range repos {
		commentsByRepo[repo] = &types.RepoComments{
			Repo: repo,
		}
	}

	q := d.commitComments().Where("Timestamp", ">=", from).OrderBy("Timestamp", fs.Asc)
	if err := d.client.IterDocs("GetCommitCommentsForRepos", from.String(), q, DEFAULT_ATTEMPTS, GET_MULTI_TIMEOUT, func(doc *fs.DocumentSnapshot) error {
		var c types.CommitComment
		if err := doc.DataTo(&c); err != nil {
			return err
		}
		if comments, ok := commentsByRepo[c.Repo]; ok {
			if comments.CommitComments == nil {
				comments.CommitComments = map[string][]*types.CommitComment{}
			}
			comments.CommitComments[c.Revision] = append(comments.CommitComments[c.Revision], &c)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	q = d.taskComments().Where("Timestamp", ">=", from).OrderBy("Timestamp", fs.Asc)
	if err := d.client.IterDocs("GetTaskCommentsForRepos", from.String(), q, DEFAULT_ATTEMPTS, GET_MULTI_TIMEOUT, func(doc *fs.DocumentSnapshot) error {
		var c types.TaskComment
		if err := doc.DataTo(&c); err != nil {
			return err
		}
		if comments, ok := commentsByRepo[c.Repo]; ok {
			if comments.TaskComments == nil {
				comments.TaskComments = map[string]map[string][]*types.TaskComment{}
			}
			byCommit, ok := comments.TaskComments[c.Revision]
			if !ok {
				byCommit = map[string][]*types.TaskComment{}
				comments.TaskComments[c.Revision] = byCommit
			}
			byCommit[c.Name] = append(byCommit[c.Name], &c)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	q = d.taskSpecComments().OrderBy("Timestamp", fs.Asc)
	if err := d.client.IterDocs("GetTaskSpecCommentsForRepos", "", q, DEFAULT_ATTEMPTS, GET_MULTI_TIMEOUT, func(doc *fs.DocumentSnapshot) error {
		var c types.TaskSpecComment
		if err := doc.DataTo(&c); err != nil {
			return err
		}
		if comments, ok := commentsByRepo[c.Repo]; ok {
			if comments.TaskSpecComments == nil {
				comments.TaskSpecComments = map[string][]*types.TaskSpecComment{}
			}
			comments.TaskSpecComments[c.Name] = append(comments.TaskSpecComments[c.Name], &c)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	rv := make([]*types.RepoComments, 0, len(repos))
	for _, repo := range repos {
		rv = append(rv, commentsByRepo[repo])
	}
	return rv, nil
}

// taskCommentId returns an ID for the TaskComment.
func taskCommentId(c *types.TaskComment) string {
	return fmt.Sprintf("%s#%s#%s#%s", url.QueryEscape(c.Repo), c.Revision, c.Name, firestore.FixTimestamp(c.Timestamp).Format(util.SAFE_TIMESTAMP_FORMAT))
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) PutTaskComment(c *types.TaskComment) error {
	c.Timestamp = firestore.FixTimestamp(c.Timestamp)
	id := taskCommentId(c)
	_, err := d.client.Create(d.taskComments().Doc(id), c, DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT)
	if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
		return db.ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	d.TrackModifiedTaskComment(c)
	return nil
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) DeleteTaskComment(c *types.TaskComment) error {
	id := taskCommentId(c)
	ref := d.taskComments().Doc(id)
	exists := true
	if _, err := d.client.Get(ref, DEFAULT_ATTEMPTS, GET_SINGLE_TIMEOUT); err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			exists = false
		} else {
			return err
		}
	}
	if exists {
		if _, err := d.client.Delete(ref, DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT); err != nil {
			return err
		}
		deleted := true
		c.Deleted = &deleted
		d.TrackModifiedTaskComment(c)
	}
	return nil
}

// taskSpecCommentId returns an ID for the TaskSpecComment.
func taskSpecCommentId(c *types.TaskSpecComment) string {
	return fmt.Sprintf("%s#%s#%s", url.QueryEscape(c.Repo), c.Name, c.Timestamp.Format(util.SAFE_TIMESTAMP_FORMAT))
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) PutTaskSpecComment(c *types.TaskSpecComment) error {
	c.Timestamp = firestore.FixTimestamp(c.Timestamp)
	id := taskSpecCommentId(c)
	_, err := d.client.Create(d.taskSpecComments().Doc(id), c, DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT)
	if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
		return db.ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	d.TrackModifiedTaskSpecComment(c)
	return nil
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) DeleteTaskSpecComment(c *types.TaskSpecComment) error {
	id := taskSpecCommentId(c)
	ref := d.taskSpecComments().Doc(id)
	exists := true
	if _, err := d.client.Get(ref, DEFAULT_ATTEMPTS, GET_SINGLE_TIMEOUT); err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			exists = false
		} else {
			return err
		}
	}
	if exists {
		if _, err := d.client.Delete(ref, DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT); err != nil {
			return err
		}
		deleted := true
		c.Deleted = &deleted
		d.TrackModifiedTaskSpecComment(c)
	}
	return nil
}

// commitCommentId returns an ID for the CommitComment.
func commitCommentId(c *types.CommitComment) string {
	return fmt.Sprintf("%s#%s#%s", url.QueryEscape(c.Repo), c.Revision, c.Timestamp.Format(util.SAFE_TIMESTAMP_FORMAT))
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) PutCommitComment(c *types.CommitComment) error {
	c.Timestamp = firestore.FixTimestamp(c.Timestamp)
	id := commitCommentId(c)
	_, err := d.client.Create(d.commitComments().Doc(id), c, DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT)
	if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
		return db.ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	d.TrackModifiedCommitComment(c)
	return nil
}

// See documentation for db.CommentDB interface.
func (d *firestoreDB) DeleteCommitComment(c *types.CommitComment) error {
	id := commitCommentId(c)
	ref := d.commitComments().Doc(id)
	exists := true
	if _, err := d.client.Get(ref, DEFAULT_ATTEMPTS, GET_SINGLE_TIMEOUT); err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			exists = false
		} else {
			return err
		}
	}
	if exists {
		if _, err := d.client.Delete(d.commitComments().Doc(id), DEFAULT_ATTEMPTS, PUT_SINGLE_TIMEOUT); err != nil {
			return err
		}
		deleted := true
		c.Deleted = &deleted
		d.TrackModifiedCommitComment(c)
	}
	return nil
}
