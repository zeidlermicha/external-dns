package provider

import (
	log "github.com/sirupsen/logrus"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"
	"github.com/zeidlermicha/shamanClient"
	"github.com/nanopack/shaman/core/common"
)

type ShamanOption func(*ShamanProvider)

func ShamanWithLogging() ShamanOption {
	return func(p *ShamanProvider) {
		p.OnApplyChanges = func(changes *plan.Changes) {
			for _, v := range changes.Create {
				log.Infof("CREATE: %v", v)
			}
			for _, v := range changes.UpdateOld {
				log.Infof("UPDATE (old): %v", v)
			}
			for _, v := range changes.UpdateNew {
				log.Infof("UPDATE (new): %v", v)
			}
			for _, v := range changes.Delete {
				log.Infof("DELETE: %v", v)
			}
		}
	}
}

func ShamanWithDomain(domainFilter DomainFilter) ShamanOption {
	return func(p *ShamanProvider) {
		p.domain = domainFilter
	}
}

type ShamanProvider struct {
	domain         DomainFilter
	client         *shamanClient.ShamanClient
	filter         *filter
	OnApplyChanges func(changes *plan.Changes)
	OnRecords      func()
}

func NewShamanProvider(host string, token string, opts ...ShamanOption) *ShamanProvider {
	shaman := &ShamanProvider{
		OnApplyChanges: func(changes *plan.Changes) {},
		OnRecords:      func() {},
		domain:         NewDomainFilter([]string{""}),
		client:         shamanClient.NewShamanClient(host, token),
	}

	for _, opt := range opts {
		opt(shaman)
	}

	return shaman
}

// Records returns the list of endpoints
func (shaman *ShamanProvider) Records() ([]*endpoint.Endpoint, error) {
	defer shaman.OnRecords()

	endpoints := make([]*endpoint.Endpoint, 0)

	records, err := shaman.client.GetRecords(&shamanClient.FullOption{ShowFull:true,})
	if err != nil {
		return nil, err
	}

	for _, resource := range records {
		for _, r := range resource.Records {
			endpoints = append(endpoints, endpoint.NewEndpointWithTTL(resource.Domain, r.Address, r.RType, endpoint.TTL(r.TTL)))
		}
	}

	return endpoints, nil
}

func (shaman *ShamanProvider) ApplyChanges(changes *plan.Changes) error {
	defer shaman.OnApplyChanges(changes)

	for _, ep := range changes.Create {

		err := shaman.create(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range changes.Delete {
		err := shaman.delete(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range changes.UpdateNew {
		err := shaman.update(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range changes.UpdateOld {
		err := shaman.update(ep)
		if err != nil {
			return err
		}
	}

	return nil
}

func (shaman *ShamanProvider) create(endpoint *endpoint.Endpoint) error {

	_, err := shaman.client.AddRecord(convertEndpoint(endpoint))
	return err
}

func (shaman *ShamanProvider) update(endpoint *endpoint.Endpoint) error {

	_, err := shaman.client.UpdateRecord(convertEndpoint(endpoint))
	return err
}

func (shaman *ShamanProvider) delete(endpoint *endpoint.Endpoint) error {

	err := shaman.client.DeleteRecord(endpoint.DNSName)
	return err
}

func convertResource(resource *common.Resource) []*endpoint.Endpoint {
	e := make([]*endpoint.Endpoint, 0)
	for _, r := range resource.Records {
		e = append(e, endpoint.NewEndpointWithTTL(resource.Domain, r.Address, r.RType, endpoint.TTL(r.TTL)))
	}

	return e
}

func convertEndpoint(endpoint *endpoint.Endpoint) *common.Resource {

	records := make([]common.Record, 0)
	for _, t := range endpoint.Targets {
		records = append(records, common.Record{Address: t, RType: endpoint.RecordType, Class: "IN", TTL: int(endpoint.RecordTTL)})
	}
	r := &common.Resource{Domain: endpoint.DNSName, Records: records}
	return r
}
