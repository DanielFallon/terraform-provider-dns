package dns

import (
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/miekg/dns"
)

func resourceDnsARecordSet() *schema.Resource {
	return &schema.Resource{
		Create: resourceDnsARecordSetCreate,
		Read:   resourceDnsARecordSetRead,
		Update: resourceDnsARecordSetUpdate,
		Delete: resourceDnsARecordSetDelete,

		Schema: map[string]*schema.Schema{
			"zone": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"addresses": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},
			"ttl": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
				Default:  3600,
			},
		},
	}
}

func resourceDnsARecordSetCreate(d *schema.ResourceData, meta interface{}) error {

	rec_name := d.Get("name").(string)
	rec_zone := d.Get("zone").(string)

	if rec_zone != dns.Fqdn(rec_zone) {
		return fmt.Errorf("Error creating DNS record: \"zone\" should be an FQDN")
	}

	rec_fqdn := fmt.Sprintf("%s.%s", rec_name, rec_zone)

	d.SetId(rec_fqdn)

	return resourceDnsARecordSetUpdate(d, meta)
}

func resourceDnsARecordSetRead(d *schema.ResourceData, meta interface{}) error {

	if meta != nil {

		rec_name := d.Get("name").(string)
		rec_zone := d.Get("zone").(string)

		if rec_zone != dns.Fqdn(rec_zone) {
			return fmt.Errorf("Error reading DNS record: \"zone\" should be an FQDN")
		}

		rec_fqdn := fmt.Sprintf("%s.%s", rec_name, rec_zone)

		c := meta.(*DNSClient).c
		srv_addr := meta.(*DNSClient).srv_addr

		msg := new(dns.Msg)
		msg.SetQuestion(rec_fqdn, dns.TypeA)

		r, _, err := c.Exchange(msg, srv_addr)
		if err != nil {
			return fmt.Errorf("Error querying DNS record: %s", err)
		}
		if r.Rcode != dns.RcodeSuccess {
			return fmt.Errorf("Error querying DNS record: %v", r.Rcode)
		}

		addresses := schema.NewSet(schema.HashString, nil)
		for _, record := range r.Answer {
			addr, err := getAVal(record)
			if err != nil {
				return fmt.Errorf("Error querying DNS record: %s", err)
			}

			// This ensures the IP address is formatted consistently
			ip := net.ParseIP(addr)
			if ip == nil {
				return fmt.Errorf("Error parsing IP address: %s", addr)
			}
			addresses.Add(ip.String())
		}

		// This ensures the IP addresses are formatted consistently
		expected := schema.NewSet(schema.HashString, nil)
		for _, addr := range d.Get("addresses").(*schema.Set).List() {
			ip := net.ParseIP(addr.(string))
			if ip == nil {
				return fmt.Errorf("Error parsing IP address: %s", addr)
			}
			expected.Add(ip.String())
		}
		if !addresses.Equal(expected) {
			d.SetId("")
			return fmt.Errorf("DNS record differs")
		}
		return nil
	} else {
		return fmt.Errorf("update server is not set")
	}
}

func resourceDnsARecordSetUpdate(d *schema.ResourceData, meta interface{}) error {

	if meta != nil {

		rec_name := d.Get("name").(string)
		rec_zone := d.Get("zone").(string)
		ttl := d.Get("ttl").(int)

		if rec_zone != dns.Fqdn(rec_zone) {
			return fmt.Errorf("Error updating DNS record: \"zone\" should be an FQDN")
		}

		rec_fqdn := fmt.Sprintf("%s.%s", rec_name, rec_zone)

		c := meta.(*DNSClient).c
		srv_addr := meta.(*DNSClient).srv_addr
		keyname := meta.(*DNSClient).keyname
		keyalgo := meta.(*DNSClient).keyalgo

		msg := new(dns.Msg)

		msg.SetUpdate(rec_zone)

		if d.HasChange("addresses") {
			o, n := d.GetChange("addresses")
			os := o.(*schema.Set)
			ns := n.(*schema.Set)
			remove := os.Difference(ns).List()
			add := ns.Difference(os).List()

			// Loop through all the old addresses and remove them
			for _, addr := range remove {
				rr_remove, _ := dns.NewRR(fmt.Sprintf("%s %d A %s", rec_fqdn, ttl, addr.(string)))
				msg.Remove([]dns.RR{rr_remove})
			}
			// Loop through all the new addresses and insert them
			for _, addr := range add {
				rr_insert, _ := dns.NewRR(fmt.Sprintf("%s %d A %s", rec_fqdn, ttl, addr.(string)))
				msg.Insert([]dns.RR{rr_insert})
			}

			if keyname != "" {
				msg.SetTsig(keyname, keyalgo, 300, time.Now().Unix())
			}

			r, _, err := c.Exchange(msg, srv_addr)
			if err != nil {
				d.SetId("")
				return fmt.Errorf("Error updating DNS record: %s", err)
			}
			if r.Rcode != dns.RcodeSuccess {
				d.SetId("")
				return fmt.Errorf("Error updating DNS record: %v", r.Rcode)
			}

			addresses := ns
			d.Set("addresses", addresses)
		}

		return resourceDnsARecordSetRead(d, meta)
	} else {
		return fmt.Errorf("update server is not set")
	}
}

func resourceDnsARecordSetDelete(d *schema.ResourceData, meta interface{}) error {

	if meta != nil {

		rec_name := d.Get("name").(string)
		rec_zone := d.Get("zone").(string)

		if rec_zone != dns.Fqdn(rec_zone) {
			return fmt.Errorf("Error updating DNS record: \"zone\" should be an FQDN")
		}

		rec_fqdn := fmt.Sprintf("%s.%s", rec_name, rec_zone)

		c := meta.(*DNSClient).c
		srv_addr := meta.(*DNSClient).srv_addr
		keyname := meta.(*DNSClient).keyname
		keyalgo := meta.(*DNSClient).keyalgo

		msg := new(dns.Msg)

		msg.SetUpdate(rec_zone)

		rr_remove, _ := dns.NewRR(fmt.Sprintf("%s 0 A", rec_fqdn))
		msg.RemoveRRset([]dns.RR{rr_remove})

		if keyname != "" {
			msg.SetTsig(keyname, keyalgo, 300, time.Now().Unix())
		}

		r, _, err := c.Exchange(msg, srv_addr)
		if err != nil {
			return fmt.Errorf("Error deleting DNS record: %s", err)
		}
		if r.Rcode != dns.RcodeSuccess {
			return fmt.Errorf("Error deleting DNS record: %v", r.Rcode)
		}

		return nil
	} else {
		return fmt.Errorf("update server is not set")
	}
}
