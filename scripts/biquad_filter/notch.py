# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Copyright (C) 2025 The OpenPSG Authors.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published
# by the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>. 

import numpy as np
import matplotlib.pyplot as plt
from scipy.signal import iirnotch, freqz

# Parameters
fs = 40.0  # Sample frequency (Hz)
f0 = 4.5   # Notch frequency (Hz)
Q = 0.5    # Quality factor - controls the width of the notch

# Generate the IIR notch filter coefficients
b, a = iirnotch(w0=f0, Q=Q, fs=fs)

# Frequency response
freq, response = freqz(b, a, worN=8000, fs=fs)

# Plot the frequency response
plt.figure(figsize=(10, 5))
plt.plot(freq, 20 * np.log10(np.abs(response)), 'b')
plt.title('Frequency Response of the IIR Notch Filter')
plt.xlabel('Frequency [Hz]')
plt.ylabel('Amplitude [dB]')
plt.grid()
plt.show()

print("IIR Notch Filter Coefficients:")
print("b (numerator):", b)
print("a (denominator):", a)
