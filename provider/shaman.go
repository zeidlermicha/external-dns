package provider

import (
	log "github.com/sirupsen/logrus"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"
	"github.com/nanopack/shaman/client"
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

func ShamanDryRun(dryRun bool) ShamanOption {
	return func(p *ShamanProvider) {
		p.dryRun = dryRun
	}
}

type ShamanProvider struct {
	domain         DomainFilter
	client         *client.ShamanClient
	filter         *filter
	dryRun         bool
	OnApplyChanges func(changes *plan.Changes)
	OnRecords      func()
}

func NewShamanProvider(host string, token string, opts ...ShamanOption) *ShamanProvider {
	shaman := &ShamanProvider{
		OnApplyChanges: func(changes *plan.Changes) {},
		OnRecords:      func() {},
		domain:         NewDomainFilter([]string{""}),
		client:         client.NewShamanClient(host, token),
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

	records, err := shaman.client.GetRecords(&client.FullOption{ShowFull: true,})
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
	if shaman.dryRun {
		return nil
	}
	for _, ep := range mergeChanges(changes.Create) {

		err := shaman.create(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range mergeChanges(changes.Delete){
		err := shaman.delete(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range mergeChanges(changes.UpdateNew) {
		err := shaman.update(ep)
		if err != nil {
			return err
		}
	}

	for _, ep := range mergeChanges(changes.UpdateOld) {
		err := shaman.update(ep)
		if err != nil {
			return err
		}
	}

	return nil
}

func mergeChanges(changes []*endpoint.Endpoint) []*common.Resource {
	m := make(map[string]*common.Resource)
	for _, e := range changes {
		if _, ok := m[e.DNSName]; !ok {
			m[e.DNSName] =&common.Resource{
				Domain:e.DNSName,
				Records:make([]common.Record,0),
			}
		}
		m[e.DNSName].Records = append(m[e.DNSName].Records, convertTargets(e)...)
	}
	values := make([]*common.Resource,len(m))
	for _, value := range m {
		values = append(values,value)
	}
	return values

}

func (shaman *ShamanProvider) create(endpoint *common.Resource) error {

	_, err := shaman.client.AddRecord(endpoint)
	return err
}

func (shaman *ShamanProvider) update(endpoint *common.Resource) error {

	_, err := shaman.client.UpdateRecord(endpoint)
	return err
}

func (shaman *ShamanProvider) delete(endpoint *common.Resource) error {

	err := shaman.client.DeleteRecord(endpoint.Domain)
	return err
}

func convertTargets(endpoint *endpoint.Endpoint) []common.Record {

	records := make([]common.Record, 0)
	for _, t := range endpoint.Targets {
		records = append(records, common.Record{Address: t, RType: endpoint.RecordType, Class: "IN", TTL: int(endpoint.RecordTTL)})
	}

	return records
}
