package git

import (
	"context"
	"fmt"
	"strings"
)

// ResolveBase returns the merge base of HEAD with the closest of the candidate branches: each is tried as a
// local branch and then as origin/<name>, and among those that exist the one with the fewest commits from its
// merge base to HEAD wins; ties keep the order of the list. It fails, naming the candidates, when none exists.
func (r *Repo) ResolveBase(ctx context.Context, candidates []string) (string, error) {
	best, bestCount := "", -1
	for _, name := range candidates {
		mb, n, ok := r.candidateBase(ctx, name)
		if ok && (bestCount < 0 || n < bestCount) {
			best, bestCount = mb, n
		}
	}
	if best == "" {
		return "", fmt.Errorf("no base branch found: tried %s, locally and on origin", strings.Join(candidates, ", "))
	}
	return best, nil
}

// candidateBase returns the merge base of HEAD with the branch name and the commits from it to HEAD, taking the
// local branch when it exists and origin/<name> otherwise. ok is false when neither exists, or when git cannot
// give the merge base or the count of the one that does.
func (r *Repo) candidateBase(ctx context.Context, name string) (mb string, n int, ok bool) {
	for _, ref := range []string{"refs/heads/" + name, "refs/remotes/origin/" + name} {
		if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			return r.distance(ctx, ref)
		}
	}
	return "", 0, false
}

// distance returns the merge base of HEAD with ref and the count of commits from it to HEAD. ok is false when git
// gives no merge base or no count.
func (r *Repo) distance(ctx context.Context, ref string) (mb string, n int, ok bool) {
	mb, err := r.git(ctx, "merge-base", "HEAD", ref)
	if err != nil || mb == "" {
		return "", 0, false
	}
	out, err := r.git(ctx, "rev-list", "--count", mb+"..HEAD")
	if err != nil {
		return "", 0, false
	}
	if _, err := fmt.Sscan(out, &n); err != nil {
		return "", 0, false
	}
	return mb, n, true
}
