package authn

const (
	MFARequiredHeader           = "X-MFA-Required"
	MFAEnrollmentRequiredHeader = "X-MFA-Enrollment-Required"
	MFATicketHeader             = "X-MFA-Ticket"
	MFAExpiresAtHeader          = "X-MFA-Expires-At"
	MFACodeHeader               = "X-MFA-Code"
	MFARecoveryCodeHeader       = "X-MFA-Recovery-Code"
)
