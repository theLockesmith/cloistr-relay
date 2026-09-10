package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PaymentsInvoicesCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_payments_invoices_created_total",
		Help: "Total number of Lightning invoices created",
	}, []string{"tier"})

	PaymentsSettled = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_payments_settled_total",
		Help: "Total number of Lightning payments settled",
	}, []string{"tier"})

	PaymentsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_payments_failed_total",
		Help: "Total number of payment failures",
	}, []string{"reason"})

	TierUpgrades = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_tier_upgrades_total",
		Help: "Total number of tier upgrades",
	}, []string{"tier"})

	TierExpirations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_tier_expirations_total",
		Help: "Total number of tier expirations (downgraded to free)",
	})

	PaymentsPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_payments_pending",
		Help: "Current number of pending Lightning invoices",
	})

	MembersJoined = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_members_joined_total",
		Help: "Total number of members joined via NIP-43",
	})
)
