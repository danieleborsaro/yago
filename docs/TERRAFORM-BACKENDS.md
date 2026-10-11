# Terraform Backends

Every Terraform configuration needs to know where its state lives. We used to write that out by hand, one
`backends/<region>.tfvars.json` per configuration, with the same bucket and KMS key copied into every one of them. Why
is that a problem? A typo in any one of them points Terraform at the wrong state, and nothing tells us.

So we declare the backend once per region with `terraform_backend`, usually in a part that every configuration of an
account already loads. yago picks the entry for `--aws-region`, works out the state key and writes
`.gitops/backend.tfvars.json`.

```yaml
# shared part, one per account
terraform_backend:
  eu-west-1:
    bucket: example-dev-statefile-euw1
    dynamodb_table: example-dev-statefile-euw1
    kms_key_id: 00000000-0000-0000-0000-000000000000
  key_vars:
    ecosystem: example-dev
```

Then each configuration simply says what it is.

```yaml
# the configuration's own part
terraform_backend:
  key_vars:
    resource_set: shared
    project: example
    component: cluster
```

The key is built from `key_pattern`, which defaults to
`{ecosystem}/{resource_set}/{project}/{component}/terraform.tfstate`. The placeholders are filled from `key_vars`, and
`{region}` and `{environment}` are always there. A `key_vars` value can be a lookup into a property another part
already holds, for example `"[[gitops.getYamlValue(project_properties_proj.project_name_short)]]"`, so the name only
lives in one place. If a placeholder has no value, yago stops. It does not guess, a half built key would essentially be
a new empty state.

Some states will not fit the pattern. Fine. Set `key` next to `key_vars` and it is used as it is. It does not go inside
the region entry, yago stops if it finds one there.

In the region entry only `bucket` is required. `region` defaults to `--aws-region` and `encrypt` to `true`, and
anything else is passed to the backend as it is - `dynamodb_table`, `use_lockfile`, whatever it might be.

When two parts set the same `terraform_backend` value, the part listed last wins, so a project part listed after a
shared one can override its `key_pattern`.

But what about the configurations we already have? A backends file listed under `wrappers.terraform.backends` for the
region is used as it is and `terraform_backend` is ignored. This way no existing state moves until we remove that
entry.

Then finally, `terraform_backend` is taken out of `.gitops/configuration.tfvars.json`, as Terraform would only warn
about a variable it doesn't know.

## The state bucket

So the backend names a bucket. But who makes the bucket? Terraform can't, it needs somewhere to keep state before it
can create anything. `yago tf backend` covers that first step, for the backend the configuration resolves to.

```bash
yago tf backend list     -d desiredstate.yaml -e dev
yago tf backend describe -d desiredstate.yaml -e dev -r eu-west-1
yago tf backend create   -d desiredstate.yaml -e dev -r eu-west-1 -n
yago tf backend create   -d desiredstate.yaml -e dev -r eu-west-1
yago tf backend update   -d desiredstate.yaml -e dev -r eu-west-1
yago tf backend delete   -d desiredstate.yaml -e dev -r eu-west-1
```

`list` takes no region. It prints the bucket and state key for every region the configuration has a backend for,
whether that comes from `terraform_backend` or from a listed backends file. `describe` prints the whole backend that
`init` would get for one region. Neither of them calls AWS or needs the Terraform code, they only read the
configuration. So they say what it points at, not whether the bucket exists. Both take `--format`, which is `table`,
`json` or `yaml`. `list` defaults to the table and `describe` to JSON. A region whose bucket isn't named yet stops
them, `init` couldn't use it either.

`create` makes the bucket in the backend's region with versioning on and all public access blocked, encrypts it, gives
it a lifecycle rule and a bucket policy, and tags it. Everything that can be checked is checked before the bucket is
made, a bucket name can't be given back. It stops if the bucket is already there. Why not simply set everything
again? Somebody may have tuned that bucket by hand, and a create that quietly resets it is not a create. That is what
`update` is for, it puts the same settings back on a bucket that exists, and stops if it doesn't.

Every S3 call that can names our own account. A bucket somebody else made under the same name is an error, never ours.

### What the bucket is called

A bucket that already exists has its name in the region entry, and that name is used as it is. But what about a
bucket that doesn't exist yet? Then the entry has no `bucket`, or there is no entry at all, and `create` names the
bucket the way the `awstagging` provider names an S3 bucket. For example
`foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8`. The parts come from the properties the
configuration already carries for that provider, `project_properties_global`, `_env` and `_proj` merged in that order,
the way a root merges them, then `project_properties`. The hash at the end is the provider's too, and
`is_create_before_destroy: false` in the properties leaves it off, as it does there. The two inputs the provider takes
per bucket go next to `key_vars`.

```yaml
terraform_backend:
  bucket_name:
    role_extend: dev
    versioning_sources: []
```

A bucket can't be renamed, so check the name with `-n` first. A long `role_extend` pushes the region code out of the
63 characters, and one without a `role` in front of it gets two hyphens. The provider does both, so we do.

Then `create` prints what it worked out, as the entry to add to the configuration. For a region that is still on a
listed backends file it prints the same two keys in that file's format and names it, the region's entry in
`terraform_backend` isn't read there. `encryption`, `bucket_name`, `lifecycle` and `policy` are, they apply to every
region. Until the name is in the configuration, `update` and `delete` work it out the same way.

```yaml
terraform_backend:
  eu-west-1:
    bucket: foo-m-management-inception-state--dev-euw1-b74d7cd3c2d40d0494c3
    kms_key_id: arn:aws:kms:eu-west-1:123456789012:key/00000000-0000-0000-0000-000000000000
```

Why not work the name out on every run and skip this step? `init`, `list` and `describe` only ever read the bucket
from the configuration. This way a property that changes later cannot quietly point every state at a new, empty
bucket.

### How it is encrypted

With a KMS key, unless the declaration says `encryption: sse`, which leaves it to S3's own key. These are the same two
words the secrets use.

```yaml
terraform_backend:
  encryption: sse
```

Which key? For a new bucket, the first of these that exists.

1. The `kms_key_id` in the region entry. It has to be a key that exists and is enabled.
2. The key of another state bucket in the same account and region. That is a bucket whose name differs from this one
   only by its `role_extend`, on an enabled key of our own account. One key per account and region is enough. A state
   bucket with a name of its own is not found this way, and neither is one whose name was cut to fit the 63
   characters. Pin its `kms_key_id` to share it.
3. A new key, with rotation on. It has no alias and no Name tag, we are avoiding naming keys.

But what about a bucket that already has a KMS key? It keeps it, and `update` does not write its encryption at all.
`update` stops if the configuration pins another key or says `sse`, yago does not change the key under existing
state. That one is left to us.

A bucket without a KMS key of its own, one a failed `create` left behind for example, is moved by `update` to what the
configuration says, which is a KMS key unless it says `sse`.
That is a change of encryption, so the plan says so in capitals and nothing happens until we agree to it. A KMS key
only works in its own region, so a bucket in a new region always gets a new key.

### How it is tagged

The same way the provider would tag it, from the same properties, and so is a key yago creates. On top of those, a
bucket and a key that yago creates get `CreatedBy: Yago`. `update` never adds that one, yago didn't make a bucket it
is only updating. If the configuration has no properties the bucket has to be named in the entry, and `CreatedBy` is
the only tag it is created with. Tags somebody added by hand are kept, ours go on top. If that leaves the bucket with
more than the 50 tags S3 allows, yago stops before changing anything. `update` only ever adds and overwrites tags, it
never removes one, so a tag the configuration no longer makes is ours to take off. The `aws:` tags on a bucket that
CloudFormation made are sent back as they are, S3 refuses a change that leaves them out, and they don't count towards
the 50.

### How the state in it is protected

Three things, and all of them have defaults, so a declaration that says nothing gets all three.

ACLs are switched off, so the account owns every object in the bucket. Uploads that bring an encryption key of their
own, SSE-C, are refused wherever yago writes the encryption.

A lifecycle rule called `expire-noncurrent-state` tidies up old versions. An old version of a state expires 90 days
after it was replaced, but the newest 10 are always kept. Uploads that were never finished go after 7 days. Delete
markers with nothing left behind them go too. But with 10 versions always kept, a state that was deleted keeps those
and its marker, so that only ever happens when no fixed number is kept.

A bucket policy with three statements. `DenyInsecureTransport` refuses every request that doesn't come over TLS.
`DenyStateDeletion` stops anybody deleting an object whose key ends in `.tfstate`. A state key that ends in anything
else is not covered by it. `DenyVersionDeletion` stops anybody deleting a version of an object. The lifecycle rule
still expires them, a bucket policy doesn't apply to that.

What does `DenyStateDeletion` cost us? `terraform workspace delete` removes the state object of that workspace, so it
is refused too. Switch `deny_state_deletion` off and take the statement off the bucket before deleting a workspace.

The numbers and the statements are settings like `encryption`, so they can be set once in a shared file and changed
for the one account, environment or service that needs something else. Only what is written is changed, the rest
keeps its default.

```yaml
terraform_backend:
  lifecycle:
    noncurrent_version_retention_days: 90
    noncurrent_versions_retained: 10
    abort_incomplete_upload_days: 7
  policy:
    deny_insecure_transport: true
    deny_state_deletion: true
    deny_version_deletion: true
```

The two numbers of days have to be at least 1, and the versions kept from 0 to 100, which is what S3 takes.
`noncurrent_versions_retained: 0` keeps no fixed number, old versions then go by age alone.

But what about a bucket that already has rules or a policy of its own? `update` keeps them. Our rule and our
statements go in by name and everything else is carried over, and the plan says how many stay. If the bucket already
has a rule or a statement under one of our names and it isn't written the same as ours, ours replaces it, and the plan
says so on that line. That is a comparison of how it is written, so one that means the same but is written differently
can be reported too. The one difference S3 makes itself, handing a list of one back as the bare value, is not. A
statement that is switched off is only left out. yago never takes one off a bucket that has it, the plan says it
stays, and that one is left to us.

An older bucket whose ACL still grants something to somebody else refuses to have its ACLs switched off. `update`
tries that before it changes anything else, so it stops there with the bucket as it was, apart from its tags.

With `DenyVersionDeletion` on, nobody can empty the bucket, us included. To delete a bucket, take that statement off
first.

### Deleting it

`delete` only deletes an empty bucket. Old versions and delete markers count, a state file somebody removed last week
can still be recovered from them. yago does not empty a bucket for us, not with `--force` either. We empty it
ourselves, on purpose, and then yago asks for the bucket name to be typed back. `-f`, `--force` skips that question
for a pipeline, nothing else. The KMS key is left alone, another bucket may be using it. The entry in the
configuration is ours to remove.

### Nothing happens unseen

Does yago ever change a bucket without telling us? No. `create` and `update` first work out everything they are
going to do, from reads alone, and print it. The bucket and its account, the key they will share or create, a change
of encryption, the lifecycle rule, the policy statements, the tags.

```
yago tf backend create will
  - create state bucket foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8 in eu-west-1, in account 123456789012
  - turn on versioning, block all public access and switch ACLs off, so the account owns every object
  - create a KMS key described as "Encrypts Terraform state, made for foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8" with rotation on, encrypt it with that key, turn on its S3 bucket key and refuse uploads that bring an encryption key of their own, SSE-C, and if any of that fails schedule the new key for deletion
  - set lifecycle rule expire-noncurrent-state, old versions of a state expire 90 days after they are replaced with the newest 10 of them always kept, delete markers with nothing left behind them are removed and unfinished uploads are aborted after 7 days
  - put these statements in its bucket policy
      DenyInsecureTransport, every request that doesn't come over TLS is refused
      DenyStateDeletion, nobody can delete an object whose key ends in .tfstate
      DenyVersionDeletion, nobody can delete a version of an object, the lifecycle rule still expires them
  - tag it with
      CreatedBy = Yago
      Foo:Business:CostCentre = cc-1234
      Foo:Business:Owner = team@example.com
      Foo:Business:Project = Example Project
      Foo:Environment:Account = Management
      Foo:Environment:AppEnvironment = Management
      Foo:Environment:InfraEnvironment = Management
      Foo:Environment:ResourceSet = Example Resource Set
      Foo:Environment:ResourceType = :AWS::S3::Bucket
      Foo:Security:Compliance = internal
      Name = foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8
  - tag the new key with
      CreatedBy = Yago
      Foo:Business:CostCentre = cc-1234
      Foo:Business:Owner = team@example.com
      Foo:Business:Project = Example Project
      Foo:Environment:Account = Management
      Foo:Environment:AppEnvironment = Management
      Foo:Environment:InfraEnvironment = Management
      Foo:Environment:ResourceSet = Example Resource Set
      Foo:Environment:ResourceType = :AWS::KMS::Key
      Foo:Security:Compliance = internal
Type yes to go ahead:
```

Only `yes` is accepted, and then exactly what was shown is done. `-A`, `--auto-approve` skips the question for a
pipeline, the plan is still printed. An `update` that would move a bucket's encryption says so in the same list, on a
line that starts `CHANGE ITS ENCRYPTION`. `delete` does the same with the bucket name in place of the yes.

All three take `-n`, `--dry-run`, or `IS_DRY_RUN=1`. A dry run prints the same plan and stops there.

What does it not do? A DynamoDB lock table, replication, policy statements of our own. Use `use_lockfile` for
locking, and once the bucket exists, import it into Terraform for the rest. The commands use `--aws-profile`, a
`role_arn` in the backend is not assumed, and yago says so when it finds one.
