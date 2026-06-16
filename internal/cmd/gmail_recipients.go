package cmd

import (
	"net/mail"
	"strings"

	"google.golang.org/api/gmail/v1"
)

type replyRecipients struct {
	To  []mail.Address
	Cc  []mail.Address
	Bcc []mail.Address
}

func buildReplyRecipients(info *replyInfo, selfEmails []string, replyAll bool, explicitTo, explicitCc, explicitBcc, remove []string) (replyRecipients, error) {
	if info == nil {
		return replyRecipients{}, usage("missing reply message")
	}

	self := make(map[string]struct{}, len(selfEmails))
	for _, email := range selfEmails {
		if key := canonicalEmail(email); key != "" {
			self[key] = struct{}{}
		}
	}

	replyTarget := parseMailboxHeader(info.ReplyToAddr)
	if len(replyTarget) == 0 {
		replyTarget = parseMailboxHeader(info.FromAddr)
	}

	var out replyRecipients
	out.To = appendMailboxes(out.To, replyTarget, self)
	if replyAll {
		out.To = appendMailboxes(out.To, parseMailboxHeader(replyToHeader(info)), self)
		out.Cc = appendMailboxes(out.Cc, parseMailboxHeader(replyCcHeader(info)), self)
	} else if len(out.To) == 0 && containsMailbox(parseMailboxHeader(info.FromAddr), self) {
		// Replying to a message sent by the active account targets its original
		// To recipients instead of producing an empty recipient list.
		out.To = appendMailboxes(out.To, parseMailboxHeader(replyToHeader(info)), self)
	}
	out = deduplicateRecipientFields(out)

	explicit, err := parseExplicitRecipientFields(explicitTo, explicitCc, explicitBcc)
	if err != nil {
		return replyRecipients{}, err
	}
	for _, placement := range []struct {
		field string
		addrs []mail.Address
	}{
		{field: "to", addrs: explicit.To},
		{field: "cc", addrs: explicit.Cc},
		{field: "bcc", addrs: explicit.Bcc},
	} {
		for _, addr := range placement.addrs {
			out.remove(addr.Address)
			switch placement.field {
			case "to":
				out.To = appendMailbox(out.To, addr)
			case "cc":
				out.Cc = appendMailbox(out.Cc, addr)
			case "bcc":
				out.Bcc = appendMailbox(out.Bcc, addr)
			}
		}
	}

	removeAddrs, err := parseMailboxValues("--remove", remove)
	if err != nil {
		return replyRecipients{}, err
	}
	for _, addr := range removeAddrs {
		out.remove(addr.Address)
	}
	out = deduplicateRecipientFields(out)

	if len(out.To)+len(out.Cc)+len(out.Bcc) == 0 {
		return replyRecipients{}, usage("reply has no recipients after applying recipient changes")
	}
	return out, nil
}

func containsMailbox(addrs []mail.Address, candidates map[string]struct{}) bool {
	for _, addr := range addrs {
		if _, ok := candidates[canonicalEmail(addr.Address)]; ok {
			return true
		}
	}
	return false
}

func parseExplicitRecipientFields(to, cc, bcc []string) (replyRecipients, error) {
	var out replyRecipients
	var err error
	if out.To, err = parseMailboxValues("--to", to); err != nil {
		return replyRecipients{}, err
	}
	if out.Cc, err = parseMailboxValues("--cc", cc); err != nil {
		return replyRecipients{}, err
	}
	if out.Bcc, err = parseMailboxValues("--bcc", bcc); err != nil {
		return replyRecipients{}, err
	}

	seen := make(map[string]string)
	for _, field := range []struct {
		name  string
		addrs []mail.Address
	}{
		{name: "--to", addrs: out.To},
		{name: "--cc", addrs: out.Cc},
		{name: "--bcc", addrs: out.Bcc},
	} {
		for _, addr := range field.addrs {
			key := canonicalEmail(addr.Address)
			if previous, ok := seen[key]; ok && previous != field.name {
				return replyRecipients{}, usagef("%s appears in both %s and %s", addr.Address, previous, field.name)
			}
			seen[key] = field.name
		}
	}
	return out, nil
}

func parseMailboxValues(flag string, values []string) ([]mail.Address, error) {
	var out []mail.Address
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		addrs, err := mail.ParseAddressList(value)
		if err != nil {
			return nil, usagef("invalid %s recipient list %q: %v", flag, value, err)
		}
		for _, addr := range addrs {
			if addr == nil || strings.TrimSpace(addr.Address) == "" {
				continue
			}
			out = appendMailbox(out, *addr)
		}
	}
	return out, nil
}

func parseMailboxHeader(value string) []mail.Address {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(value)
	if err == nil {
		out := make([]mail.Address, 0, len(addrs))
		for _, addr := range addrs {
			if addr != nil && strings.TrimSpace(addr.Address) != "" {
				out = appendMailbox(out, *addr)
			}
		}
		return out
	}

	fallback := parseEmailAddressesFallback(value)
	out := make([]mail.Address, 0, len(fallback))
	for _, email := range fallback {
		out = appendMailbox(out, mail.Address{Address: email})
	}
	return out
}

func appendMailboxes(dst []mail.Address, src []mail.Address, excluded map[string]struct{}) []mail.Address {
	for _, addr := range src {
		if _, skip := excluded[canonicalEmail(addr.Address)]; skip {
			continue
		}
		dst = appendMailbox(dst, addr)
	}
	return dst
}

func appendMailbox(dst []mail.Address, addr mail.Address) []mail.Address {
	key := canonicalEmail(addr.Address)
	if key == "" {
		return dst
	}
	addr.Address = strings.TrimSpace(addr.Address)
	addr.Name = strings.TrimSpace(addr.Name)
	for i := range dst {
		if canonicalEmail(dst[i].Address) != key {
			continue
		}
		if dst[i].Name == "" && addr.Name != "" {
			dst[i] = addr
		}
		return dst
	}
	return append(dst, addr)
}

func deduplicateRecipientFields(in replyRecipients) replyRecipients {
	var out replyRecipients
	seen := make(map[string]struct{})
	for _, field := range []struct {
		src  []mail.Address
		dest *[]mail.Address
	}{
		{src: in.To, dest: &out.To},
		{src: in.Cc, dest: &out.Cc},
		{src: in.Bcc, dest: &out.Bcc},
	} {
		for _, addr := range field.src {
			key := canonicalEmail(addr.Address)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			*field.dest = append(*field.dest, addr)
		}
	}
	return out
}

func (r *replyRecipients) remove(email string) {
	key := canonicalEmail(email)
	r.To = removeMailbox(r.To, key)
	r.Cc = removeMailbox(r.Cc, key)
	r.Bcc = removeMailbox(r.Bcc, key)
}

func removeMailbox(addrs []mail.Address, key string) []mail.Address {
	out := addrs[:0]
	for _, addr := range addrs {
		if canonicalEmail(addr.Address) != key {
			out = append(out, addr)
		}
	}
	return out
}

func canonicalEmail(value string) string {
	value = strings.TrimSpace(value)
	if addr, err := mail.ParseAddress(value); err == nil && addr != nil {
		value = addr.Address
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func formatMailboxes(addrs []mail.Address) []string {
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if strings.TrimSpace(addr.Address) == "" {
			continue
		}
		if strings.TrimSpace(addr.Name) == "" {
			out = append(out, addr.Address)
		} else {
			out = append(out, addr.String())
		}
	}
	return out
}

func selfEmailsForReply(account, sendingEmail string, sendAs []*gmail.SendAs) []string {
	out := []string{account, sendingEmail}
	for _, alias := range sendAs {
		if alias != nil {
			out = append(out, alias.SendAsEmail)
		}
	}
	return out
}

func replyModeName(replyAll bool) string {
	if replyAll {
		return "reply-all"
	}
	return "reply"
}

func replyToHeader(info *replyInfo) string {
	if strings.TrimSpace(info.ToHeader) != "" {
		return info.ToHeader
	}
	return strings.Join(info.ToAddrs, ", ")
}

func replyCcHeader(info *replyInfo) string {
	if strings.TrimSpace(info.CcHeader) != "" {
		return info.CcHeader
	}
	return strings.Join(info.CcAddrs, ", ")
}
