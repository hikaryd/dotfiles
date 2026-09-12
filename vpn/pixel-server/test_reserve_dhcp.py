import unittest
from unittest.mock import patch

from reserve_dhcp import check_reservation, phone_identity


class ReservationTests(unittest.TestCase):
    def setUp(self):
        self.mac = '02:00:00:00:00:01'
        self.ip = '192.0.2.116'
        self.lease = {'mac': self.mac, 'ip': self.ip}
        self.pools = {'home': {'enable': True, 'range': {
            'begin': '192.0.2.3', 'end': '192.0.2.122'}}}

    def check(self, leases=None, reservations=None, pools=None):
        return check_reservation(self.mac, self.ip,
                                 [self.lease] if leases is None else leases,
                                 [] if reservations is None else reservations,
                                 self.pools if pools is None else pools)

    def test_new(self):
        self.assertFalse(self.check())

    def test_idempotent(self):
        self.assertTrue(self.check(reservations=[self.lease]))

    def test_missing_lease(self):
        with self.assertRaises(RuntimeError):
            self.check(leases=[])

    def test_conflicting_mac_or_ip(self):
        for conflict in [{'mac': self.mac, 'ip': '192.0.2.5'},
                         {'mac': '00:11:22:33:44:55', 'ip': self.ip}]:
            with self.assertRaises(RuntimeError):
                self.check(reservations=[conflict])

    def test_no_pool_or_ambiguous(self):
        for pools in [{}, dict(self.pools, other=self.pools['home'])]:
            with self.assertRaises(RuntimeError):
                self.check(pools=pools)

    @patch('reserve_dhcp.subprocess.check_output')
    def test_phone_identity(self, command):
        command.return_value = f'MAC: {self.mac}, IP: /{self.ip}, Supplicant state: COMPLETED'
        self.assertEqual(phone_identity('test'), (self.mac, self.ip))
        command.return_value = 'Wifi is disabled'
        with self.assertRaises(RuntimeError):
            phone_identity('test')


if __name__ == '__main__':
    unittest.main()
