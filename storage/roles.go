package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WebIdentity is the keys of a role assumed with a web identity's token,
// read from a file, as EKS gives a pod's service account a role, by STS's
// AssumeRoleWithWebIdentity: a call that no keys sign, as the token says
// who's asking. The file is read again at each fetch, as the cluster turns
// the token over, and each set of keys lasts STS's hour.
//
// A WebIdentity keeps the keys it fetched, so it's one for the app, not one
// a request.
type WebIdentity struct {
	// RoleARN is the role to assume.
	RoleARN string

	// TokenFile is the file the token is in.
	TokenFile string

	// SessionName names the session in AWS's logs: tug's and the time
	// unless set.
	SessionName string

	// Region is STS's region, whose endpoint is asked: the global one,
	// sts.amazonaws.com, unless set.
	Region string

	// Endpoint is STS's address, as a test's: the Region's unless set.
	Endpoint string

	// Client makes the requests: http.DefaultClient unless set.
	Client *http.Client

	fetched fetched
	now     func() time.Time
}

// Keys are the role's keys, fetched from STS when they're due.
func (w *WebIdentity) Keys(ctx context.Context) (Keys, error) {
	return w.fetched.get(ctx, clockOf(w.now), w.fetch)
}

func (w *WebIdentity) fetch(ctx context.Context) (Keys, error) {
	endpoint := w.Endpoint
	switch {
	case endpoint != "":
	case w.Region != "":
		endpoint = "https://sts." + w.Region + ".amazonaws.com/"
	default:
		endpoint = "https://sts.amazonaws.com/"
	}
	failed := func(err error) (Keys, error) {
		return Keys{}, fmt.Errorf("storage: no keys from STS at %s for %s: %w", endpoint, w.RoleARN, err)
	}
	token, err := os.ReadFile(w.TokenFile)
	if err != nil {
		return failed(fmt.Errorf("reading the web identity's token: %w", err))
	}
	session := w.SessionName
	if session == "" {
		session = "tug-" + strconv.FormatInt(clockOf(w.now)().UnixNano(), 10)
	}
	form := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {"2011-06-15"},
		"RoleArn":          {w.RoleARN},
		"RoleSessionName":  {session},
		"WebIdentityToken": {strings.TrimSpace(string(token))},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return failed(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := w.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return failed(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return failed(err)
	}
	if resp.StatusCode != http.StatusOK {
		var refused struct {
			Error struct {
				Code    string `xml:"Code"`
				Message string `xml:"Message"`
			} `xml:"Error"`
		}
		if xml.Unmarshal(body, &refused) == nil && refused.Error.Code != "" {
			return failed(fmt.Errorf("%s: %s", refused.Error.Code, refused.Error.Message))
		}
		return failed(errors.New(resp.Status))
	}
	var assumed struct {
		Credentials struct {
			AccessKeyID     string    `xml:"AccessKeyId"`
			SecretAccessKey string    `xml:"SecretAccessKey"`
			SessionToken    string    `xml:"SessionToken"`
			Expiration      time.Time `xml:"Expiration"`
		} `xml:"AssumeRoleWithWebIdentityResult>Credentials"`
	}
	if err := xml.Unmarshal(body, &assumed); err != nil {
		return failed(fmt.Errorf("reading the answer: %w", err))
	}
	c := assumed.Credentials
	return keysOf(c.AccessKeyID, c.SecretAccessKey, c.SessionToken, c.Expiration, failed)
}

// ContainerRole is the keys a container's endpoint gives, as ECS gives a
// task's at 169.254.170.2, by the path it puts in the task's environment,
// and EKS's Pod Identity gives a pod's at 169.254.170.23, with a token it
// takes as their Authorization, read from a file at each fetch, as EKS
// turns it over.
//
// A ContainerRole keeps the keys it fetched, so it's one for the app, not
// one a request.
type ContainerRole struct {
	// URL is the endpoint's address. Over plain HTTP, it's the machine
	// itself, ECS's 169.254.170.2, or EKS's 169.254.170.23 or fd00:ec2::23,
	// as AWS's SDKs have it: an endpoint anywhere else would be handed the
	// token.
	URL string

	// Token is the Authorization the endpoint takes.
	Token string

	// TokenFile is a file the Authorization is read from at each fetch,
	// in place of Token.
	TokenFile string

	// Client makes the requests: unless set, one that takes no proxy, as
	// the endpoint is the machine's own and the keys would cross it, and
	// waits two seconds for an answer.
	Client *http.Client

	fetched fetched
	now     func() time.Time
}

// Keys are the container's keys, fetched from its endpoint when they're
// due.
func (c *ContainerRole) Keys(ctx context.Context) (Keys, error) {
	return c.fetched.get(ctx, clockOf(c.now), c.fetch)
}

func (c *ContainerRole) fetch(ctx context.Context) (Keys, error) {
	failed := func(err error) (Keys, error) {
		return Keys{}, fmt.Errorf("storage: no keys from the container's endpoint at %s: %w", c.URL, err)
	}
	if err := containerEndpoint(ctx, c.URL); err != nil {
		return failed(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return failed(err)
	}
	auth := c.Token
	if c.TokenFile != "" {
		token, err := os.ReadFile(c.TokenFile)
		if err != nil {
			return failed(fmt.Errorf("reading its token: %w", err))
		}
		auth = strings.TrimSpace(string(token))
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := c.Client
	if client == nil {
		client = localClient(2 * time.Second)
	}
	resp, err := client.Do(req)
	if err != nil {
		return failed(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failed(answered(resp))
	}
	var given struct {
		AccessKeyID     string    `json:"AccessKeyId"`
		SecretAccessKey string    `json:"SecretAccessKey"`
		Token           string    `json:"Token"`
		Expiration      time.Time `json:"Expiration"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&given); err != nil {
		return failed(fmt.Errorf("reading the answer: %w", err))
	}
	return keysOf(given.AccessKeyID, given.SecretAccessKey, given.Token, given.Expiration, failed)
}

// The addresses a container's endpoint may be at over plain HTTP, but
// for the machine's own: ECS's, and EKS's in IPv4 and IPv6.
var (
	ecsEndpoint  = netip.MustParseAddr("169.254.170.2")
	eksEndpoint  = netip.MustParseAddr("169.254.170.23")
	eksEndpoint6 = netip.MustParseAddr("fd00:ec2::23")
)

// containerEndpoint checks that the endpoint at link may be handed its
// token: over HTTPS, anywhere, and over plain HTTP, only at the machine
// itself, by each address its name has, or at ECS's or EKS's.
func containerEndpoint(ctx context.Context, link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("it isn't an http:// or https:// address")
	}
	if u.Scheme == "https" {
		return nil
	}
	host := u.Hostname()
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else if ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host); err != nil {
		return err
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		if !ip.IsLoopback() && ip != ecsEndpoint && ip != eksEndpoint && ip != eksEndpoint6 {
			return fmt.Errorf("over plain HTTP, it may only be the machine itself, ECS's 169.254.170.2, or EKS's 169.254.170.23 or fd00:ec2::23, which %s isn't", host)
		}
	}
	return nil
}

// InstanceRole is the keys of the role an EC2 instance has, from its
// metadata, by IMDSv2: a token from a PUT, then the role's name, and its
// keys, each asked for with the token. IMDSv1, which answers a plain GET,
// as a server tricked into fetching a URL makes, isn't asked.
//
// A container on the instance gets IMDSv2's token only when the instance
// lets the answer cross two hops, its HttpPutResponseHopLimit of 2.
//
// An InstanceRole keeps the keys it fetched, so it's one for the app, not
// one a request.
type InstanceRole struct {
	// Endpoint is the metadata's address: http://169.254.169.254, or
	// http://[fd00:ec2::254] on IPv6, unless set.
	Endpoint string

	// Client makes the requests: unless set, one that takes no proxy, as
	// the metadata is the instance's own and the keys would cross it, and
	// waits a second for each answer, as AWS's SDKs do.
	Client *http.Client

	fetched fetched
	now     func() time.Time
}

// Keys are the instance's keys, fetched from its metadata when they're
// due.
func (i *InstanceRole) Keys(ctx context.Context) (Keys, error) {
	return i.fetched.get(ctx, clockOf(i.now), i.fetch)
}

func (i *InstanceRole) fetch(ctx context.Context) (Keys, error) {
	endpoint := strings.TrimSuffix(i.Endpoint, "/")
	if endpoint == "" {
		endpoint = "http://169.254.169.254"
	}
	failed := func(err error) (Keys, error) {
		return Keys{}, fmt.Errorf("storage: no keys from the instance's metadata at %s: %w", endpoint, err)
	}
	client := i.Client
	if client == nil {
		client = localClient(time.Second)
	}
	ask := func(method, path string, header http.Header) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, endpoint+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header = header
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("it didn't answer: %w", err)
		}
		return resp, nil
	}

	resp, err := ask(http.MethodPut, "/latest/api/token", http.Header{"X-Aws-Ec2-Metadata-Token-Ttl-Seconds": {"300"}})
	if err != nil {
		return failed(err)
	}
	token, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()
	if err != nil {
		return failed(err)
	}
	if resp.StatusCode != http.StatusOK {
		return failed(fmt.Errorf("a token for IMDSv2: %w", answered(resp)))
	}
	withToken := http.Header{"X-Aws-Ec2-Metadata-Token": {string(token)}}

	const credentials = "/latest/meta-data/iam/security-credentials/"
	resp, err = ask(http.MethodGet, credentials, withToken)
	if err != nil {
		return failed(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return failed(errors.New("the instance has no role"))
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return failed(fmt.Errorf("the role's name: %w", answered(resp)))
	}
	names := bufio.NewScanner(io.LimitReader(resp.Body, 4<<10))
	names.Scan()
	role := strings.TrimSpace(names.Text())
	resp.Body.Close()
	if role == "" {
		return failed(errors.New("the instance has no role"))
	}

	resp, err = ask(http.MethodGet, credentials+url.PathEscape(role), withToken)
	if err != nil {
		return failed(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failed(fmt.Errorf("the keys of the role %s: %w", role, answered(resp)))
	}
	var given struct {
		Code            string    `json:"Code"`
		AccessKeyID     string    `json:"AccessKeyId"`
		SecretAccessKey string    `json:"SecretAccessKey"`
		Token           string    `json:"Token"`
		Expiration      time.Time `json:"Expiration"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&given); err != nil {
		return failed(fmt.Errorf("reading the keys of the role %s: %w", role, err))
	}
	if given.Code != "Success" {
		return failed(fmt.Errorf("the keys of the role %s: %s", role, given.Code))
	}
	return keysOf(given.AccessKeyID, given.SecretAccessKey, given.Token, given.Expiration, failed)
}

// keysOf is the keys a source was given, or an error by failed when they
// aren't whole: a role's are temporary, with a session token and an
// expiry.
func keysOf(id, secret, token string, expires time.Time, failed func(error) (Keys, error)) (Keys, error) {
	if id == "" || secret == "" || token == "" || expires.IsZero() {
		return failed(errors.New("the answer has no keys, or not all of them"))
	}
	return Keys{AccessKeyID: id, SecretAccessKey: secret, SessionToken: token, Expires: expires}, nil
}

// answered is an endpoint's answer that wasn't the keys, as an error: its
// status, and the start of what it said.
func answered(resp *http.Response) error {
	said, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	if s := strings.Join(strings.Fields(string(said)), " "); s != "" {
		return fmt.Errorf("%s: %s", resp.Status, s)
	}
	return errors.New(resp.Status)
}

// localClients are the clients of an endpoint on the machine itself, by
// how long each waits for an answer: they take no proxy, as the keys the
// endpoint gives would cross it.
var localClients sync.Map // time.Duration → *http.Client

func localClient(timeout time.Duration) *http.Client {
	if c, ok := localClients.Load(timeout); ok {
		return c.(*http.Client)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	c, _ := localClients.LoadOrStore(timeout, &http.Client{Transport: transport, Timeout: timeout})
	return c.(*http.Client)
}

// RoleFromEnv is the KeySource of the role the environment says the app
// runs as, where AWS's SDKs look for one, in their order:
//
//   - a web identity's, as EKS gives a pod's service account, by
//     AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN, named by
//     AWS_ROLE_SESSION_NAME, with STS in AWS_REGION, or
//     AWS_DEFAULT_REGION, or at AWS_ENDPOINT_URL_STS;
//   - a container's, as ECS gives a task, by
//     AWS_CONTAINER_CREDENTIALS_RELATIVE_URI, or as EKS's Pod Identity
//     gives a pod, by AWS_CONTAINER_CREDENTIALS_FULL_URI, with
//     AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE or
//     AWS_CONTAINER_AUTHORIZATION_TOKEN;
//   - and else an EC2 instance's, from its metadata at
//     AWS_EC2_METADATA_SERVICE_ENDPOINT, or by
//     AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE=IPv6 at its IPv6 address,
//     unless AWS_EC2_METADATA_DISABLED is true, when there's none, and
//     it's nil.
//
// What doesn't add up, as a role with no token, is an error.
func RoleFromEnv() (KeySource, error) {
	if file := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"); file != "" {
		role := os.Getenv("AWS_ROLE_ARN")
		if role == "" {
			return nil, errors.New("storage: AWS_WEB_IDENTITY_TOKEN_FILE is set, and AWS_ROLE_ARN, the role it's for, isn't")
		}
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
		return &WebIdentity{
			RoleARN:     role,
			TokenFile:   file,
			SessionName: os.Getenv("AWS_ROLE_SESSION_NAME"),
			Region:      region,
			Endpoint:    os.Getenv("AWS_ENDPOINT_URL_STS"),
		}, nil
	}
	if path := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"); path != "" {
		return &ContainerRole{URL: "http://" + ecsEndpoint.String() + path}, nil
	}
	if link := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI"); link != "" {
		u, err := url.Parse(link)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("storage: AWS_CONTAINER_CREDENTIALS_FULL_URI is %q, which isn't an http:// or https:// address", link)
		}
		return &ContainerRole{
			URL:       link,
			Token:     os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN"),
			TokenFile: os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"),
		}, nil
	}
	if v := os.Getenv("AWS_EC2_METADATA_DISABLED"); strings.EqualFold(v, "true") {
		return nil, nil
	}
	endpoint := os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT")
	if endpoint == "" {
		switch mode := os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE"); {
		case mode == "" || strings.EqualFold(mode, "IPv4"):
		case strings.EqualFold(mode, "IPv6"):
			endpoint = "http://[fd00:ec2::254]"
		default:
			return nil, fmt.Errorf("storage: AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE is %q, which isn't IPv4 or IPv6", mode)
		}
	}
	return &InstanceRole{Endpoint: endpoint}, nil
}
