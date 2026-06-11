package config

// Default values stamped onto a freshly registered registry.ProfileRef and
// the fallback slugs used when a profile, project, or asset name slugifies
// to the empty string.

// DefaultProfileSource is the value stamped onto registry.ProfileRef.Source
// when a profile is created or registered locally.
const DefaultProfileSource = "local"

// DefaultProfileManagedBy is the value stamped onto
// registry.ProfileRef.ManagedBy when a profile is created or registered
// locally (as opposed to being managed by an external system).
const DefaultProfileManagedBy = "self"

// DefaultProfileSlug is the fallback id used by profile.slug when the input
// name produces an empty slug.
const DefaultProfileSlug = "profile"

// DefaultProjectSlug is the fallback id used when a project name slugifies
// to the empty string.
const DefaultProjectSlug = "project"

// DefaultAssetSlug is the fallback id used when an asset name slugifies to
// the empty string.
const DefaultAssetSlug = "asset"
